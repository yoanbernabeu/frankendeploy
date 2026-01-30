package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

var rollbackCmd = &cobra.Command{
	Use:   "rollback <server> [release]",
	Short: "Rollback to previous release",
	Long: `Rolls back the application to a previous release.

If no release is specified, rolls back to the immediately previous release.

Example:
  frankendeploy rollback production           # Rollback to previous
  frankendeploy rollback production 20240115  # Rollback to specific release`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runRollback,
}

func init() {
	rootCmd.AddCommand(rollbackCmd)
}

func runRollback(cmd *cobra.Command, args []string) error {
	serverName := args[0]
	targetRelease := ""
	if len(args) > 1 {
		targetRelease = args[1]
	}

	// Validate inputs
	if err := security.ValidateServerName(serverName); err != nil {
		return fmt.Errorf("invalid server name: %w", err)
	}
	if targetRelease != "" {
		if err := security.ValidateRelease(targetRelease); err != nil {
			return fmt.Errorf("invalid release name: %w", err)
		}
	}

	// Load project config
	projectCfg, err := config.LoadProjectConfig(GetConfigFile())
	if err != nil {
		return err
	}

	// Load global config
	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	// Get server config
	serverCfg, err := globalCfg.GetServer(serverName)
	if err != nil {
		return err
	}

	appPath := filepath.Join("/opt/frankendeploy/apps", projectCfg.Name)

	PrintInfo("Connecting to %s...", serverCfg.Host)

	client := ssh.NewClient(serverCfg.Host, serverCfg.User, serverCfg.Port, serverCfg.KeyPath)
	if err := client.Connect(); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer client.Close()

	// Get current release
	result, _ := client.Exec(fmt.Sprintf("readlink %s/current | xargs basename", appPath))
	currentRelease := strings.TrimSpace(result.Stdout)

	if targetRelease == "" {
		// Get previous release
		result, err := client.Exec(fmt.Sprintf("ls -1t %s/releases | head -2 | tail -1", appPath))
		if err != nil {
			return fmt.Errorf("failed to get releases: %w", err)
		}
		targetRelease = strings.TrimSpace(result.Stdout)

		if targetRelease == "" || targetRelease == currentRelease {
			return fmt.Errorf("no previous release available")
		}
	}

	// Verify target release exists
	releasePath := filepath.Join(appPath, "releases", targetRelease)
	result, _ = client.Exec(fmt.Sprintf("test -d %s && echo 'exists'", releasePath))
	if !strings.Contains(result.Stdout, "exists") {
		// List available releases
		result, _ = client.Exec(fmt.Sprintf("ls -1t %s/releases", appPath))
		return fmt.Errorf("release '%s' not found. Available releases:\n%s", targetRelease, result.Stdout)
	}

	PrintInfo("Rolling back from %s to %s...", currentRelease, targetRelease)

	// Stop current container
	_, _ = client.Exec(fmt.Sprintf("docker stop %s 2>/dev/null || true", projectCfg.Name))
	_, _ = client.Exec(fmt.Sprintf("docker rm %s 2>/dev/null || true", projectCfg.Name))

	// Find the image for the target release
	imageName := fmt.Sprintf("%s:%s", projectCfg.Name, targetRelease)

	// Start the old container with shared .env.local mounted
	// SECURITY: Run as non-root user (1000:1000) with non-privileged port 8080
	startCmd := fmt.Sprintf(`docker run -d --name %s \
		--network frankendeploy \
		--restart unless-stopped \
		--user 1000:1000 \
		-e SERVER_NAME=:8080 \
		-e APP_ENV=prod \
		-e APP_DEBUG=0 \
		-v %s/shared/.env.local:/app/.env.local:ro \
		%s`, projectCfg.Name, appPath, imageName)

	result, err = client.Exec(startCmd)
	if err != nil || result.ExitCode != 0 {
		return fmt.Errorf("failed to start container: %s", result.Stderr)
	}

	// Update current symlink
	currentPath := filepath.Join(appPath, "current")
	_, _ = client.Exec(fmt.Sprintf("ln -sfn %s %s", releasePath, currentPath))

	PrintSuccess("Rolled back to release %s", targetRelease)

	return nil
}
