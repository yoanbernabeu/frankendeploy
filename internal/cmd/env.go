package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/constants"
	"github.com/yoanbernabeu/frankendeploy/internal/deploy"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
	"golang.org/x/term"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage environment variables on servers",
	Long:  `Commands to manage environment variables for your application on remote servers.`,
}

var (
	envSetReload    bool
	envSetFromStdin bool
)

var envSetCmd = &cobra.Command{
	Use:   "set <server> <KEY=value | KEY --from-stdin>",
	Short: "Set an environment variable",
	Long: `Sets an environment variable on the server.

For secrets, prefer --from-stdin: the value never appears in your shell
history nor in the remote command line. Interactively, the value is asked
with a hidden prompt; in scripts, it is read from stdin.

Use --reload to apply changes immediately (restarts the container).
Without --reload, changes take effect on next deployment.

Example:
  frankendeploy env set prod DATABASE_URL="postgresql://user:pass@host/db"
  frankendeploy env set prod APP_SECRET --from-stdin
  openssl rand -hex 32 | frankendeploy env set prod APP_SECRET --from-stdin
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
	envSetCmd.Flags().BoolVar(&envSetFromStdin, "from-stdin", false, "Read the value from stdin (keeps secrets out of shell history)")
	envPushCmd.Flags().BoolVar(&envPushReload, "reload", false, "Restart container to apply changes immediately")
}

func runEnvSet(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	serverName := args[0]

	key, value, err := resolveEnvSetInput(args[1], envSetFromStdin, os.Stdin)
	if err != nil {
		return err
	}

	conn, err := ConnectToServer(serverName)
	if err != nil {
		return err
	}
	defer conn.Client.Close()

	// Read, update, write through the unified writer (chmod 600 + chown)
	envVars, err := deploy.ReadEnvVars(ctx, conn.Client, conn.Project.Name)
	if err != nil {
		return err
	}
	envVars[key] = value
	if err := deploy.WriteEnvVars(ctx, conn.Client, conn.Project.Name, envVars); err != nil {
		return err
	}

	PrintSuccess("Set %s on %s", key, serverName)

	// Reload container if requested
	if envSetReload {
		if err := reloadContainer(ctx, conn.Client, conn.Project); err != nil {
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

	envVars := deploy.ParseEnvContent(result.Stdout)
	keys := make([]string, 0, len(envVars))
	for key := range envVars {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := envVars[key]
		// Mask sensitive values
		displayValue := value
		if security.IsSensitiveEnvKey(key) && len(value) > 8 {
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

	result, err := conn.Client.Exec(ctx, fmt.Sprintf("cat %s 2>/dev/null", envFile))
	if err != nil {
		return fmt.Errorf("failed to read env file: %w", err)
	}
	envVars := deploy.ParseEnvContent(result.Stdout)

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

	envVars, err := deploy.ReadEnvVars(ctx, conn.Client, conn.Project.Name)
	if err != nil {
		return err
	}

	if _, ok := envVars[key]; !ok {
		return fmt.Errorf("variable %s not found", key)
	}

	delete(envVars, key)

	if err := deploy.WriteEnvVars(ctx, conn.Client, conn.Project.Name, envVars); err != nil {
		return err
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

	// Read existing and merge (new values override existing), then write
	// through the unified writer (chmod 600 + chown)
	existingVars, err := deploy.ReadEnvVars(ctx, conn.Client, conn.Project.Name)
	if err != nil {
		return err
	}
	newVars := deploy.ParseEnvContent(string(content))
	for key, value := range newVars {
		existingVars[key] = value
	}
	if err := deploy.WriteEnvVars(ctx, conn.Client, conn.Project.Name, existingVars); err != nil {
		return err
	}

	PrintSuccess("Pushed %d variables from %s to %s", len(newVars), localFile, serverName)

	// Reload container if requested
	if envPushReload {
		if err := reloadContainer(ctx, conn.Client, conn.Project); err != nil {
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

// resolveEnvSetInput turns the `env set` argument into a (key, value) pair.
// With fromStdin, the argument is the bare key and the value is read from
// stdin: a hidden prompt on a terminal, raw stdin otherwise — either way the
// secret stays out of shell history and remote command lines.
func resolveEnvSetInput(arg string, fromStdin bool, stdin *os.File) (string, string, error) {
	if !fromStdin {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid format, use KEY=value (or KEY --from-stdin)")
		}
		if err := security.ValidateEnvKey(parts[0]); err != nil {
			return "", "", fmt.Errorf("invalid environment variable key: %w", err)
		}
		return parts[0], parts[1], nil
	}

	if strings.Contains(arg, "=") {
		return "", "", fmt.Errorf("with --from-stdin, pass the key only (got %q)", arg)
	}
	if err := security.ValidateEnvKey(arg); err != nil {
		return "", "", fmt.Errorf("invalid environment variable key: %w", err)
	}

	if term.IsTerminal(int(stdin.Fd())) {
		fmt.Printf("Enter value for %s (input hidden): ", arg)
		value, err := term.ReadPassword(int(stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", "", fmt.Errorf("failed to read value: %w", err)
		}
		return arg, string(value), nil
	}

	value, err := readStdinValue(stdin)
	if err != nil {
		return "", "", err
	}
	return arg, value, nil
}

// readStdinValue reads a piped value from stdin, trimming the trailing
// newline that `echo` or `openssl rand` append.
func readStdinValue(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("failed to read value from stdin: %w", err)
	}
	value := strings.TrimRight(string(data), "\r\n")
	if value == "" {
		return "", fmt.Errorf("empty value on stdin")
	}
	return value, nil
}

// reloadContainer performs a rolling restart to apply env changes without
// downtime. It reuses the deploy primitives: same docker run command (mounts,
// managed DATABASE_URL, restart policy) and the same rename-based swap.
func reloadContainer(ctx context.Context, client ssh.Executor, cfg *config.ProjectConfig) error {
	appName := cfg.Name
	PrintInfo("Reloading container...")

	// Get current container info
	result, err := client.Exec(ctx, fmt.Sprintf("docker inspect %s --format '{{.Config.Image}}'", appName))
	if err != nil || result == nil || result.ExitCode != 0 {
		return fmt.Errorf("container not running")
	}
	imageName := strings.TrimSpace(result.Stdout)

	appPath := constants.AppBasePath(appName)
	tempName := appName + "-new"

	// Start new container with updated env (same command as deploy/rollback)
	databaseURL := readSavedDatabaseURL(ctx, client, appPath)
	if err := startNewContainer(ctx, client, cfg, imageName, appPath, "", databaseURL, tempName); err != nil {
		return err
	}

	// Wait for new container to be healthy
	PrintInfo("Waiting for new container to be ready...")
	for i := 0; i < 30; i++ {
		status := "starting"
		if result, err := client.Exec(ctx, fmt.Sprintf("docker inspect %s --format '{{.State.Health.Status}}' 2>/dev/null || echo 'starting'", tempName)); err == nil && result != nil {
			status = strings.TrimSpace(result.Stdout)
		}
		if status == "healthy" {
			break
		}
		if i == 29 {
			forceRemoveContainer(ctx, client, tempName)
			return fmt.Errorf("new container failed health check")
		}
		if _, err := client.Exec(ctx, "sleep 2"); err != nil {
			PrintVerbose("Sleep interrupted: %v", err)
		}
	}

	// Zero-downtime name handover with restore on failure (same as deploy)
	if err := swapContainerNames(ctx, client, appName, tempName, true); err != nil {
		forceRemoveContainer(ctx, client, tempName)
		return err
	}

	return nil
}
