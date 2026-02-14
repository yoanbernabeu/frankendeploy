package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/frankendeploy/internal/constants"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
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
	ctx := cmd.Context()
	serverName := args[0]
	targetRelease := ""
	if len(args) > 1 {
		targetRelease = args[1]
	}

	// Validate release name if provided
	if targetRelease != "" {
		if err := security.ValidateRelease(targetRelease); err != nil {
			return fmt.Errorf("invalid release name: %w", err)
		}
	}

	conn, err := ConnectToServer(serverName)
	if err != nil {
		return err
	}
	defer conn.Client.Close()

	appPath := constants.AppBasePath(conn.Project.Name)

	PrintInfo("Connecting to %s...", conn.Server.Host)

	// Get current release
	result, _ := conn.Client.Exec(ctx, fmt.Sprintf("readlink %s/current | xargs basename", appPath))
	currentRelease := strings.TrimSpace(result.Stdout)

	if targetRelease == "" {
		// Get previous release
		result, err := conn.Client.Exec(ctx, fmt.Sprintf("ls -1t %s/releases | head -2 | tail -1", appPath))
		if err != nil {
			return fmt.Errorf("failed to get releases: %w", err)
		}
		targetRelease = strings.TrimSpace(result.Stdout)

		if targetRelease == "" || targetRelease == currentRelease {
			return fmt.Errorf("no previous release available")
		}
	}

	// Verify target release exists
	releasePath := constants.AppReleasePath(conn.Project.Name, targetRelease)
	result, _ = conn.Client.Exec(ctx, fmt.Sprintf("test -d %s && echo 'exists'", releasePath))
	if !strings.Contains(result.Stdout, "exists") {
		// List available releases
		result, _ = conn.Client.Exec(ctx, fmt.Sprintf("ls -1t %s/releases", appPath))
		return fmt.Errorf("release '%s' not found. Available releases:\n%s", targetRelease, result.Stdout)
	}

	PrintInfo("Rolling back from %s to %s...", currentRelease, targetRelease)

	// Blue-green rollback: start new container first, then stop old
	imageName := fmt.Sprintf("%s:%s", conn.Project.Name, targetRelease)
	tempName := conn.Project.Name + "-rollback"

	// Start the target release container with a temporary name
	startCmd := fmt.Sprintf(`docker run -d --name %s \
		--network %s \
		--restart unless-stopped \
		--user %s \
		-e SERVER_NAME=:%s \
		-e APP_ENV=prod \
		-e APP_DEBUG=0 \
		-v %s/shared/.env.local:/app/.env.local:ro \
		%s`, tempName, constants.NetworkName, constants.ContainerUser, constants.AppPort, appPath, imageName)

	result, err = conn.Client.Exec(ctx, startCmd)
	if err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}
	if err := result.Err(); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Stop old container and rename new one
	stopAndRemoveContainer(ctx, conn.Client, conn.Project.Name)
	if _, err := conn.Client.Exec(ctx, fmt.Sprintf("docker rename %s %s", tempName, conn.Project.Name)); err != nil {
		PrintWarning("Failed to rename container: %v", err)
	}

	// Update current symlink
	currentPath := constants.AppCurrentPath(conn.Project.Name)
	if _, err := conn.Client.Exec(ctx, fmt.Sprintf("ln -sfn %s %s", releasePath, currentPath)); err != nil {
		PrintWarning("Failed to update symlink: %v", err)
	}

	PrintSuccess("Rolled back to release %s", targetRelease)

	return nil
}
