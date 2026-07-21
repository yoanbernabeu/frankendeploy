package deploy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
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
func CheckEnvVars(ctx context.Context, client ssh.Executor, cfg *config.ProjectConfig, serverName string) (*EnvCheckResult, error) {
	result := &EnvCheckResult{
		Missing:   []EnvRequirement{},
		Present:   []string{},
		Generated: make(map[string]string),
	}

	// Read existing env variables
	existingVars, err := ReadEnvVars(ctx, client, cfg.Name)
	if err != nil {
		return nil, err
	}

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
	if cfg.Database.IsManaged() {
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
func SaveGeneratedSecrets(ctx context.Context, client ssh.Executor, appName string, secrets map[string]string) error {
	if len(secrets) == 0 {
		return nil
	}

	// Read existing env file and merge new secrets
	existingVars, err := ReadEnvVars(ctx, client, appName)
	if err != nil {
		return err
	}
	for key, value := range secrets {
		existingVars[key] = value
	}

	return WriteEnvVars(ctx, client, appName, existingVars)
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

