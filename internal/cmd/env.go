package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/frankendeploy/internal/config"
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

func getEnvFilePath(appName string) string {
	return fmt.Sprintf("/opt/frankendeploy/apps/%s/shared/.env.local", appName)
}

func connectToServer(serverName string) (*ssh.Client, *config.ProjectConfig, error) {
	// Load project config
	projectCfg, err := config.LoadProjectConfig(GetConfigFile())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load project config: %w", err)
	}

	// Load global config
	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load global config: %w", err)
	}

	// Get server config
	serverCfg, err := globalCfg.GetServer(serverName)
	if err != nil {
		return nil, nil, err
	}

	// Connect via SSH
	client := ssh.NewClient(serverCfg.Host, serverCfg.User, serverCfg.Port, serverCfg.KeyPath)
	if err := client.Connect(); err != nil {
		return nil, nil, fmt.Errorf("failed to connect: %w", err)
	}

	return client, projectCfg, nil
}

func runEnvSet(cmd *cobra.Command, args []string) error {
	serverName := args[0]
	keyValue := args[1]

	// Validate server name
	if err := security.ValidateServerName(serverName); err != nil {
		return fmt.Errorf("invalid server name: %w", err)
	}

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

	client, projectCfg, err := connectToServer(serverName)
	if err != nil {
		return err
	}
	defer client.Close()

	envFile := getEnvFilePath(projectCfg.Name)

	// Ensure directory exists
	mkdirCmd := fmt.Sprintf("mkdir -p $(dirname %s)", envFile)
	_, _ = client.Exec(mkdirCmd)

	// Read existing env file
	result, _ := client.Exec(fmt.Sprintf("cat %s 2>/dev/null || echo ''", envFile))
	existingContent := result.Stdout

	// Parse existing variables
	envVars := parseEnvContent(existingContent)

	// Set/update the variable
	envVars[key] = value

	// Write back
	newContent := buildEnvContent(envVars)
	writeCmd := fmt.Sprintf("cat > %s << 'ENVEOF'\n%sENVEOF", envFile, newContent)
	if _, err := client.Exec(writeCmd); err != nil {
		return fmt.Errorf("failed to write env file: %w", err)
	}

	PrintSuccess("Set %s on %s", key, serverName)

	// Reload container if requested
	if envSetReload {
		if err := reloadContainer(client, projectCfg.Name); err != nil {
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
	serverName := args[0]

	// Validate server name
	if err := security.ValidateServerName(serverName); err != nil {
		return fmt.Errorf("invalid server name: %w", err)
	}

	client, projectCfg, err := connectToServer(serverName)
	if err != nil {
		return err
	}
	defer client.Close()

	envFile := getEnvFilePath(projectCfg.Name)

	result, err := client.Exec(fmt.Sprintf("cat %s 2>/dev/null", envFile))
	if err != nil || result.Stdout == "" {
		PrintInfo("No environment variables configured on %s", serverName)
		return nil
	}

	fmt.Printf("Environment variables for %s on %s:\n\n", projectCfg.Name, serverName)

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
	serverName := args[0]
	key := args[1]

	// Validate inputs
	if err := security.ValidateServerName(serverName); err != nil {
		return fmt.Errorf("invalid server name: %w", err)
	}
	if err := security.ValidateEnvKey(key); err != nil {
		return fmt.Errorf("invalid environment variable key: %w", err)
	}

	client, projectCfg, err := connectToServer(serverName)
	if err != nil {
		return err
	}
	defer client.Close()

	envFile := getEnvFilePath(projectCfg.Name)

	result, _ := client.Exec(fmt.Sprintf("cat %s 2>/dev/null", envFile))
	envVars := parseEnvContent(result.Stdout)

	if value, ok := envVars[key]; ok {
		fmt.Println(value)
	} else {
		return fmt.Errorf("variable %s not found", key)
	}

	return nil
}

func runEnvRemove(cmd *cobra.Command, args []string) error {
	serverName := args[0]
	key := args[1]

	// Validate inputs
	if err := security.ValidateServerName(serverName); err != nil {
		return fmt.Errorf("invalid server name: %w", err)
	}
	if err := security.ValidateEnvKey(key); err != nil {
		return fmt.Errorf("invalid environment variable key: %w", err)
	}

	client, projectCfg, err := connectToServer(serverName)
	if err != nil {
		return err
	}
	defer client.Close()

	envFile := getEnvFilePath(projectCfg.Name)

	result, _ := client.Exec(fmt.Sprintf("cat %s 2>/dev/null", envFile))
	envVars := parseEnvContent(result.Stdout)

	if _, ok := envVars[key]; !ok {
		return fmt.Errorf("variable %s not found", key)
	}

	delete(envVars, key)

	newContent := buildEnvContent(envVars)
	writeCmd := fmt.Sprintf("cat > %s << 'ENVEOF'\n%sENVEOF", envFile, newContent)
	if _, err := client.Exec(writeCmd); err != nil {
		return fmt.Errorf("failed to write env file: %w", err)
	}

	PrintSuccess("Removed %s from %s", key, serverName)
	return nil
}

func runEnvPush(cmd *cobra.Command, args []string) error {
	serverName := args[0]
	localFile := args[1]

	// Validate server name
	if err := security.ValidateServerName(serverName); err != nil {
		return fmt.Errorf("invalid server name: %w", err)
	}

	// Read local file
	content, err := os.ReadFile(localFile)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", localFile, err)
	}

	client, projectCfg, err := connectToServer(serverName)
	if err != nil {
		return err
	}
	defer client.Close()

	envFile := getEnvFilePath(projectCfg.Name)

	// Ensure directory exists
	mkdirCmd := fmt.Sprintf("mkdir -p $(dirname %s)", envFile)
	_, _ = client.Exec(mkdirCmd)

	// Read existing and merge
	result, _ := client.Exec(fmt.Sprintf("cat %s 2>/dev/null || echo ''", envFile))
	existingVars := parseEnvContent(result.Stdout)
	newVars := parseEnvContent(string(content))

	// Merge (new values override existing)
	for key, value := range newVars {
		existingVars[key] = value
	}

	// Write merged content
	mergedContent := buildEnvContent(existingVars)
	writeCmd := fmt.Sprintf("cat > %s << 'ENVEOF'\n%sENVEOF", envFile, mergedContent)
	if _, err := client.Exec(writeCmd); err != nil {
		return fmt.Errorf("failed to write env file: %w", err)
	}

	PrintSuccess("Pushed %d variables from %s to %s", len(newVars), localFile, serverName)

	// Reload container if requested
	if envPushReload {
		if err := reloadContainer(client, projectCfg.Name); err != nil {
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
	serverName := args[0]

	// Validate server name
	if err := security.ValidateServerName(serverName); err != nil {
		return fmt.Errorf("invalid server name: %w", err)
	}

	client, projectCfg, err := connectToServer(serverName)
	if err != nil {
		return err
	}
	defer client.Close()

	envFile := getEnvFilePath(projectCfg.Name)

	result, err := client.Exec(fmt.Sprintf("cat %s 2>/dev/null", envFile))
	if err != nil || result.Stdout == "" {
		PrintInfo("No environment variables to pull from %s", serverName)
		return nil
	}

	// Write to local file
	localFile := fmt.Sprintf(".env.%s.backup", serverName)
	if err := os.WriteFile(localFile, []byte(result.Stdout), 0600); err != nil {
		return fmt.Errorf("failed to write %s: %w", localFile, err)
	}

	PrintSuccess("Pulled environment variables to %s", localFile)
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
func reloadContainer(client *ssh.Client, appName string) error {
	PrintInfo("Reloading container...")

	// Get current container info
	result, err := client.Exec(fmt.Sprintf("docker inspect %s --format '{{.Config.Image}}'", appName))
	if err != nil || result.ExitCode != 0 {
		return fmt.Errorf("container not running")
	}
	imageName := strings.TrimSpace(result.Stdout)

	appPath := fmt.Sprintf("/opt/frankendeploy/apps/%s", appName)
	tempName := appName + "-new"

	// Start new container with updated env
	// SECURITY: Run as non-root user (1000:1000) with non-privileged port 8080
	startCmd := fmt.Sprintf(`docker run -d --name %s \
		--network frankendeploy \
		--user 1000:1000 \
		-e SERVER_NAME=:8080 \
		-e APP_ENV=prod \
		-e APP_DEBUG=0 \
		-v %s/shared/.env.local:/app/.env.local:ro \
		%s`, tempName, appPath, imageName)

	result, err = client.Exec(startCmd)
	if err != nil || result.ExitCode != 0 {
		return fmt.Errorf("failed to start new container: %s", result.Stderr)
	}

	// Wait for new container to be healthy
	PrintInfo("Waiting for new container to be ready...")
	for i := 0; i < 30; i++ {
		result, _ := client.Exec(fmt.Sprintf("docker inspect %s --format '{{.State.Health.Status}}' 2>/dev/null || echo 'starting'", tempName))
		status := strings.TrimSpace(result.Stdout)
		if status == "healthy" {
			break
		}
		if i == 29 {
			// Cleanup and fail
			_, _ = client.Exec(fmt.Sprintf("docker rm -f %s", tempName))
			return fmt.Errorf("new container failed health check")
		}
		_, _ = client.Exec("sleep 2")
	}

	// Stop old container and rename new one
	_, _ = client.Exec(fmt.Sprintf("docker stop %s", appName))
	_, _ = client.Exec(fmt.Sprintf("docker rm %s", appName))
	_, _ = client.Exec(fmt.Sprintf("docker rename %s %s", tempName, appName))

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
