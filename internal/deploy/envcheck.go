package deploy

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

// EnvRequirement defines a required environment variable
type EnvRequirement struct {
	Name         string
	Description  string
	CanGenerate  bool
	DefaultValue string
}

// EnvCheckResult holds the result of environment variable checking
type EnvCheckResult struct {
	Missing   []EnvRequirement
	Present   []string
	Generated map[string]string
}

// FrameworkEnvRequirements defines required variables by framework
var FrameworkEnvRequirements = map[string][]EnvRequirement{
	"symfony": {
		{Name: "APP_SECRET", Description: "Symfony security secret", CanGenerate: true},
		{Name: "APP_ENV", Description: "Application environment", DefaultValue: "prod"},
	},
}

// CheckEnvVars verifies that required environment variables are set on the server
func CheckEnvVars(client *ssh.Client, cfg *config.ProjectConfig, serverName string) (*EnvCheckResult, error) {
	result := &EnvCheckResult{
		Missing:   []EnvRequirement{},
		Present:   []string{},
		Generated: make(map[string]string),
	}

	// Get the env file path
	envFile := filepath.Join("/opt/frankendeploy/apps", cfg.Name, "shared", ".env.local")

	// Read existing env variables
	execResult, _ := client.Exec(fmt.Sprintf("cat %s 2>/dev/null || echo ''", envFile))
	existingVars := parseEnvContent(execResult.Stdout)

	// Get framework requirements (default to Symfony)
	requirements := FrameworkEnvRequirements["symfony"]

	// Check each required variable
	for _, req := range requirements {
		if value, exists := existingVars[req.Name]; exists && value != "" {
			result.Present = append(result.Present, req.Name)
		} else if req.DefaultValue != "" {
			// Has a default value, so it's optional
			result.Present = append(result.Present, req.Name)
		} else {
			result.Missing = append(result.Missing, req)
		}
	}

	// Check DATABASE_URL if needed
	if needsDatabaseURL(cfg) {
		if value, exists := existingVars["DATABASE_URL"]; exists && value != "" {
			result.Present = append(result.Present, "DATABASE_URL")
		} else {
			result.Missing = append(result.Missing, EnvRequirement{
				Name:        "DATABASE_URL",
				Description: "Database connection URL (required - database is not managed)",
				CanGenerate: false,
			})
		}
	}

	return result, nil
}

// needsDatabaseURL determines if DATABASE_URL is required based on config
func needsDatabaseURL(cfg *config.ProjectConfig) bool {
	// No database configured
	if cfg.Database.Driver == "" {
		return false
	}

	// Managed mode: FrankenDeploy provides DATABASE_URL
	if cfg.Database.Managed != nil && *cfg.Database.Managed {
		return false
	}

	// SQLite: no DATABASE_URL needed (file-based database)
	if cfg.Database.Driver == "sqlite" || cfg.Database.Driver == "pdo_sqlite" {
		return false
	}

	// External database: DATABASE_URL required
	return true
}

// GenerateSymfonySecret generates a secure 64-character hex secret
func GenerateSymfonySecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// GenerateMissingSecrets generates values for variables that can be auto-generated
func GenerateMissingSecrets(missing []EnvRequirement) (map[string]string, error) {
	generated := make(map[string]string)

	for _, req := range missing {
		if !req.CanGenerate {
			continue
		}

		switch req.Name {
		case "APP_SECRET":
			secret, err := GenerateSymfonySecret()
			if err != nil {
				return nil, fmt.Errorf("failed to generate APP_SECRET: %w", err)
			}
			generated["APP_SECRET"] = secret
		}
	}

	return generated, nil
}

// SaveGeneratedSecrets writes generated secrets to the server's .env.local file
func SaveGeneratedSecrets(client *ssh.Client, appName string, secrets map[string]string) error {
	if len(secrets) == 0 {
		return nil
	}

	envFile := filepath.Join("/opt/frankendeploy/apps", appName, "shared", ".env.local")

	// Ensure directory exists
	mkdirCmd := fmt.Sprintf("mkdir -p $(dirname %s)", envFile)
	_, _ = client.Exec(mkdirCmd)

	// Read existing env file
	result, _ := client.Exec(fmt.Sprintf("cat %s 2>/dev/null || echo ''", envFile))
	existingVars := parseEnvContent(result.Stdout)

	// Merge new secrets
	for key, value := range secrets {
		existingVars[key] = value
	}

	// Write back
	newContent := buildEnvContent(existingVars)
	writeCmd := fmt.Sprintf("cat > %s << 'ENVEOF'\n%sENVEOF", envFile, newContent)
	if _, err := client.Exec(writeCmd); err != nil {
		return fmt.Errorf("failed to write env file: %w", err)
	}

	// Fix permissions for container user 1000:1000
	_, _ = client.Exec(fmt.Sprintf("sudo chown 1000:1000 %s 2>/dev/null || true", envFile))
	_, _ = client.Exec(fmt.Sprintf("sudo chmod 600 %s 2>/dev/null || true", envFile))

	return nil
}

// FormatEnvCheckError formats an error message with commands to run
func FormatEnvCheckError(missing []EnvRequirement, serverName string) string {
	var sb strings.Builder

	sb.WriteString("Missing required environment variables:\n\n")

	for _, req := range missing {
		sb.WriteString(fmt.Sprintf("   %s (%s)\n", req.Name, req.Description))
	}

	sb.WriteString("\nRun the following commands to configure them:\n\n")

	for _, req := range missing {
		switch req.Name {
		case "APP_SECRET":
			sb.WriteString(fmt.Sprintf("   frankendeploy env set %s APP_SECRET=$(openssl rand -hex 32)\n", serverName))
		case "DATABASE_URL":
			sb.WriteString(fmt.Sprintf("   frankendeploy env set %s DATABASE_URL=\"postgresql://user:pass@host:5432/db\"\n", serverName))
		default:
			sb.WriteString(fmt.Sprintf("   frankendeploy env set %s %s=\"<value>\"\n", serverName, req.Name))
		}
	}

	sb.WriteString(fmt.Sprintf("\nThen run 'frankendeploy deploy %s' again.\n", serverName))
	sb.WriteString("\nOr use --force to skip this check (not recommended for production)")

	return sb.String()
}

// parseEnvContent parses .env file content into a map
func parseEnvContent(content string) map[string]string {
	vars := make(map[string]string)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Parse KEY=value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			// Remove quotes if present
			value = strings.Trim(value, "\"'")
			vars[key] = value
		}
	}

	return vars
}

// buildEnvContent builds .env file content from a map
func buildEnvContent(vars map[string]string) string {
	var lines []string
	for key, value := range vars {
		// Quote values with spaces or special characters
		if strings.ContainsAny(value, " \t\n\"'") {
			value = fmt.Sprintf("\"%s\"", value)
		}
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}
	return strings.Join(lines, "\n") + "\n"
}
