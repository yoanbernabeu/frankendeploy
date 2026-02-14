package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/frankendeploy/internal/constants"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage environment variables on servers",
	Long:  `Commands to manage environment variables for your application on remote servers.`,
}

var envSetReload bool

var envSetCmd = &cobra.Command{
	Use:   "set <server> <KEY=value>",
	Short: "Set an environment variable",
	Long: `Sets an environment variable on the server.

Use --reload to apply changes immediately (restarts the container).
Without --reload, changes take effect on next deployment.

Example:
  frankendeploy env set prod DATABASE_URL="postgresql://user:pass@host/db"
  frankendeploy env set prod APP_SECRET="my-secret-key" --reload`,
	Args: cobra.ExactArgs(2),
	RunE: runEnvSet,
}

var envListCmd = &cobra.Command{
	Use:   "list <server>",
	Short: "List environment variables",
	Long: `Lists all environment variables configured on the server.

Example:
  frankendeploy env list prod`,
	Args: cobra.ExactArgs(1),
	RunE: runEnvList,
}

var envGetCmd = &cobra.Command{
	Use:   "get <server> <KEY>",
	Short: "Get an environment variable",
	Long: `Gets the value of an environment variable from the server.

Example:
  frankendeploy env get prod DATABASE_URL`,
	Args: cobra.ExactArgs(2),
	RunE: runEnvGet,
}

var envRemoveCmd = &cobra.Command{
	Use:   "remove <server> <KEY>",
	Short: "Remove an environment variable",
	Long: `Removes an environment variable from the server.

Example:
  frankendeploy env remove prod DATABASE_URL`,
	Args: cobra.ExactArgs(2),
	RunE: runEnvRemove,
}

var envPushReload bool

var envPushCmd = &cobra.Command{
	Use:   "push <server> <file>",
	Short: "Push a .env file to the server",
	Long: `Pushes a local .env file to the server, merging with existing variables.

Use --reload to apply changes immediately (restarts the container).

Example:
  frankendeploy env push prod .env.prod --reload`,
	Args: cobra.ExactArgs(2),
	RunE: runEnvPush,
}

var envPullCmd = &cobra.Command{
	Use:   "pull <server>",
	Short: "Pull environment variables from the server",
	Long: `Downloads the environment variables from the server to a local file.

Example:
  frankendeploy env pull prod`,
	Args: cobra.ExactArgs(1),
	RunE: runEnvPull,
}

func init() {
	rootCmd.AddCommand(envCmd)
	envCmd.AddCommand(envSetCmd)
	envCmd.AddCommand(envListCmd)
	envCmd.AddCommand(envGetCmd)
	envCmd.AddCommand(envRemoveCmd)
	envCmd.AddCommand(envPushCmd)
	envCmd.AddCommand(envPullCmd)

	envSetCmd.Flags().BoolVar(&envSetReload, "reload", false, "Restart container to apply changes immediately")
	envPushCmd.Flags().BoolVar(&envPushReload, "reload", false, "Restart container to apply changes immediately")
}

func runEnvSet(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	serverName := args[0]
	keyValue := args[1]

	// Parse KEY=value
	parts := strings.SplitN(keyValue, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid format, use KEY=value")
	}
	key, value := parts[0], parts[1]

	// Validate environment variable key
	if err := security.ValidateEnvKey(key); err != nil {
		return fmt.Errorf("invalid environment variable key: %w", err)
	}
	_ = value // Value is not validated as it can contain any content

	conn, err := ConnectToServer(serverName)
	if err != nil {
		return err
	}
	defer conn.Client.Close()

	envFile := constants.AppEnvFilePath(conn.Project.Name)

	// Ensure directory exists
	mkdirCmd := fmt.Sprintf("mkdir -p $(dirname %s)", envFile)
	if _, err := conn.Client.Exec(ctx, mkdirCmd); err != nil {
		PrintWarning("Could not create directory: %v", err)
	}

	// Read existing env file
	result, _ := conn.Client.Exec(ctx, fmt.Sprintf("cat %s 2>/dev/null || echo ''", envFile))
	existingContent := result.Stdout

	// Parse existing variables
	envVars := parseEnvContent(existingContent)

	// Set/update the variable
	envVars[key] = value

	// Write back
	newContent := buildEnvContent(envVars)
	delim, err := security.GenerateHeredocDelimiter("ENVEOF")
	if err != nil {
		return fmt.Errorf("failed to generate delimiter: %w", err)
	}
	writeCmd := fmt.Sprintf("cat > %s << '%s'\n%s%s", envFile, delim, newContent, delim)
	if _, err := conn.Client.Exec(ctx, writeCmd); err != nil {
		return fmt.Errorf("failed to write env file: %w", err)
	}

	PrintSuccess("Set %s on %s", key, serverName)

	// Reload container if requested
	if envSetReload {
		if err := reloadContainer(ctx, conn.Client, conn.Project.Name); err != nil {
			PrintWarning("Failed to reload: %v", err)
			PrintInfo("Changes will take effect on next deployment")
		} else {
			PrintSuccess("Container reloaded - changes applied")
		}
	} else {
		PrintInfo("Run with --reload to apply immediately, or deploy to apply changes")
	}

	return nil
}

func runEnvList(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	serverName := args[0]

	conn, err := ConnectToServer(serverName)
	if err != nil {
		return err
	}
	defer conn.Client.Close()

	envFile := constants.AppEnvFilePath(conn.Project.Name)

	result, err := conn.Client.Exec(ctx, fmt.Sprintf("cat %s 2>/dev/null", envFile))
	if err != nil || result.Stdout == "" {
		PrintInfo("No environment variables configured on %s", serverName)
		return nil
	}

	fmt.Printf("Environment variables for %s on %s:\n\n", conn.Project.Name, serverName)

	envVars := parseEnvContent(result.Stdout)
	for key, value := range envVars {
		// Mask sensitive values
		displayValue := value
		if isSensitiveKey(key) && len(value) > 8 {
			displayValue = value[:4] + "****" + value[len(value)-4:]
		}
		fmt.Printf("  %s=%s\n", key, displayValue)
	}

	return nil
}

func runEnvGet(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	serverName := args[0]
	key := args[1]

	// Validate env key
	if err := security.ValidateEnvKey(key); err != nil {
		return fmt.Errorf("invalid environment variable key: %w", err)
	}

	conn, err := ConnectToServer(serverName)
	if err != nil {
		return err
	}
	defer conn.Client.Close()

	envFile := constants.AppEnvFilePath(conn.Project.Name)

	result, _ := conn.Client.Exec(ctx, fmt.Sprintf("cat %s 2>/dev/null", envFile))
	envVars := parseEnvContent(result.Stdout)

	if value, ok := envVars[key]; ok {
		fmt.Println(value)
	} else {
		return fmt.Errorf("variable %s not found", key)
	}

	return nil
}

func runEnvRemove(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	serverName := args[0]
	key := args[1]

	// Validate env key
	if err := security.ValidateEnvKey(key); err != nil {
		return fmt.Errorf("invalid environment variable key: %w", err)
	}

	conn, err := ConnectToServer(serverName)
	if err != nil {
		return err
	}
	defer conn.Client.Close()

	envFile := constants.AppEnvFilePath(conn.Project.Name)

	result, _ := conn.Client.Exec(ctx, fmt.Sprintf("cat %s 2>/dev/null", envFile))
	envVars := parseEnvContent(result.Stdout)

	if _, ok := envVars[key]; !ok {
		return fmt.Errorf("variable %s not found", key)
	}

	delete(envVars, key)

	newContent := buildEnvContent(envVars)
	delim, err := security.GenerateHeredocDelimiter("ENVEOF")
	if err != nil {
		return fmt.Errorf("failed to generate delimiter: %w", err)
	}
	writeCmd := fmt.Sprintf("cat > %s << '%s'\n%s%s", envFile, delim, newContent, delim)
	if _, err := conn.Client.Exec(ctx, writeCmd); err != nil {
		return fmt.Errorf("failed to write env file: %w", err)
	}

	PrintSuccess("Removed %s from %s", key, serverName)
	return nil
}

func runEnvPush(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	serverName := args[0]
	localFile := args[1]

	// Validate local file: must be a .env file
	baseName := filepath.Base(localFile)
	if !strings.HasPrefix(baseName, ".env") {
		return fmt.Errorf("only .env files can be pushed (got: %s)", baseName)
	}

	// Validate local file is within the current working directory
	absLocalFile, err := filepath.Abs(localFile)
	if err != nil {
		return fmt.Errorf("failed to resolve file path: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	if !strings.HasPrefix(absLocalFile, cwd+string(filepath.Separator)) && absLocalFile != cwd {
		return fmt.Errorf("file must be within the project directory")
	}

	// Read local file
	content, err := os.ReadFile(localFile)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", localFile, err)
	}

	conn, err := ConnectToServer(serverName)
	if err != nil {
		return err
	}
	defer conn.Client.Close()

	envFile := constants.AppEnvFilePath(conn.Project.Name)

	// Ensure directory exists
	mkdirCmd := fmt.Sprintf("mkdir -p $(dirname %s)", envFile)
	if _, err := conn.Client.Exec(ctx, mkdirCmd); err != nil {
		PrintWarning("Could not create directory: %v", err)
	}

	// Read existing and merge
	result, _ := conn.Client.Exec(ctx, fmt.Sprintf("cat %s 2>/dev/null || echo ''", envFile))
	existingVars := parseEnvContent(result.Stdout)
	newVars := parseEnvContent(string(content))

	// Merge (new values override existing)
	for key, value := range newVars {
		existingVars[key] = value
	}

	// Write merged content
	mergedContent := buildEnvContent(existingVars)
	delim, err := security.GenerateHeredocDelimiter("ENVEOF")
	if err != nil {
		return fmt.Errorf("failed to generate delimiter: %w", err)
	}
	writeCmd := fmt.Sprintf("cat > %s << '%s'\n%s%s", envFile, delim, mergedContent, delim)
	if _, err := conn.Client.Exec(ctx, writeCmd); err != nil {
		return fmt.Errorf("failed to write env file: %w", err)
	}

	PrintSuccess("Pushed %d variables from %s to %s", len(newVars), localFile, serverName)

	// Reload container if requested
	if envPushReload {
		if err := reloadContainer(ctx, conn.Client, conn.Project.Name); err != nil {
			PrintWarning("Failed to reload: %v", err)
			PrintInfo("Changes will take effect on next deployment")
		} else {
			PrintSuccess("Container reloaded - changes applied")
		}
	} else {
		PrintInfo("Run with --reload to apply immediately, or deploy to apply changes")
	}

	return nil
}

func runEnvPull(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	serverName := args[0]

	conn, err := ConnectToServer(serverName)
	if err != nil {
		return err
	}
	defer conn.Client.Close()

	envFile := constants.AppEnvFilePath(conn.Project.Name)

	result, err := conn.Client.Exec(ctx, fmt.Sprintf("cat %s 2>/dev/null", envFile))
	if err != nil || result.Stdout == "" {
		PrintInfo("No environment variables to pull from %s", serverName)
		return nil
	}

	// Write to local file
	localFileOut := fmt.Sprintf(".env.%s.backup", serverName)
	if err := os.WriteFile(localFileOut, []byte(result.Stdout), 0600); err != nil {
		return fmt.Errorf("failed to write %s: %w", localFileOut, err)
	}

	PrintSuccess("Pulled environment variables to %s", localFileOut)
	PrintWarning("This file contains secrets - do not commit to git!")
	return nil
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

// reloadContainer performs a rolling restart to apply env changes with minimal downtime
func reloadContainer(ctx context.Context, client *ssh.Client, appName string) error {
	PrintInfo("Reloading container...")

	// Get current container info
	result, err := client.Exec(ctx, fmt.Sprintf("docker inspect %s --format '{{.Config.Image}}'", appName))
	if err != nil || result.ExitCode != 0 {
		return fmt.Errorf("container not running")
	}
	imageName := strings.TrimSpace(result.Stdout)

	appPath := constants.AppBasePath(appName)
	tempName := appName + "-new"

	// Start new container with updated env
	startCmd := fmt.Sprintf(`docker run -d --name %s \
		--network %s \
		--user %s \
		-e SERVER_NAME=:%s \
		-e APP_ENV=prod \
		-e APP_DEBUG=0 \
		-v %s/shared/.env.local:/app/.env.local:ro \
		%s`, tempName, constants.NetworkName, constants.ContainerUser, constants.AppPort, appPath, imageName)

	result, err = client.Exec(ctx, startCmd)
	if err != nil {
		return fmt.Errorf("failed to start new container: %w", err)
	}
	if err := result.Err(); err != nil {
		return fmt.Errorf("failed to start new container: %w", err)
	}

	// Wait for new container to be healthy
	PrintInfo("Waiting for new container to be ready...")
	for i := 0; i < 30; i++ {
		result, _ := client.Exec(ctx, fmt.Sprintf("docker inspect %s --format '{{.State.Health.Status}}' 2>/dev/null || echo 'starting'", tempName))
		status := strings.TrimSpace(result.Stdout)
		if status == "healthy" {
			break
		}
		if i == 29 {
			// Cleanup and fail
			if _, err := client.Exec(ctx, fmt.Sprintf("docker rm -f %s", tempName)); err != nil {
				PrintWarning("Failed to cleanup temporary container: %v", err)
			}
			return fmt.Errorf("new container failed health check")
		}
		if _, err := client.Exec(ctx, "sleep 2"); err != nil {
			PrintVerbose("Sleep interrupted: %v", err)
		}
	}

	// Stop old container and rename new one
	if _, err := client.Exec(ctx, fmt.Sprintf("docker stop %s", appName)); err != nil {
		PrintWarning("Failed to stop old container: %v", err)
	}
	if _, err := client.Exec(ctx, fmt.Sprintf("docker rm %s", appName)); err != nil {
		PrintWarning("Failed to remove old container: %v", err)
	}
	if _, err := client.Exec(ctx, fmt.Sprintf("docker rename %s %s", tempName, appName)); err != nil {
		PrintWarning("Failed to rename container: %v", err)
	}

	return nil
}

// isSensitiveKey checks if a key likely contains sensitive data
func isSensitiveKey(key string) bool {
	sensitivePatterns := []string{
		"SECRET", "PASSWORD", "PASS", "KEY", "TOKEN",
		"DATABASE_URL", "MAILER_DSN", "API_KEY",
	}
	keyUpper := strings.ToUpper(key)
	for _, pattern := range sensitivePatterns {
		if strings.Contains(keyUpper, pattern) {
			return true
		}
	}
	return false
}
