package cmd

import (
	"fmt"
	"sort"
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

The rollback uses the same pipeline as a deployment: the target release is
started with the same mounts and environment (including the managed database
URL), health checked, and swapped in without downtime. If the health check
fails, the current version keeps running.

Example:
  frankendeploy rollback production           # Rollback to previous
  frankendeploy rollback production 20240115  # Rollback to specific release`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runRollback,
}

func init() {
	rootCmd.AddCommand(rollbackCmd)
}

// findPreviousRelease returns the release that precedes current in reverse
// lexicographic order (release tags default to sortable timestamps). It
// ignores the current release itself and returns an error when there is
// nothing to roll back to.
func findPreviousRelease(releases []string, current string) (string, error) {
	sorted := make([]string, 0, len(releases))
	for _, r := range releases {
		if r != "" {
			sorted = append(sorted, r)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(sorted)))

	currentKnown := false
	for _, r := range sorted {
		if r == current {
			currentKnown = true
			break
		}
	}

	if currentKnown {
		// Newest release strictly older than the current one — a second
		// rollback must not bounce forward to a newer release.
		for _, r := range sorted {
			if r < current {
				return r, nil
			}
		}
		return "", fmt.Errorf("no previous release available")
	}

	// Current release unknown (broken symlink?): newest available release.
	if len(sorted) > 0 {
		return sorted[0], nil
	}
	return "", fmt.Errorf("no previous release available")
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
	client := conn.Client
	cfg := conn.Project
	appName := cfg.Name
	appPath := constants.AppBasePath(appName)

	PrintInfo("Connecting to %s...", conn.Server.Host)

	// Get current release
	currentRelease := ""
	if result, err := client.Exec(ctx, fmt.Sprintf("readlink %s/current | xargs basename", appPath)); err == nil && result != nil {
		currentRelease = strings.TrimSpace(result.Stdout)
	}

	if targetRelease == "" {
		listResult, err := client.Exec(ctx, fmt.Sprintf("ls -1 %s/releases", appPath))
		if err != nil {
			return fmt.Errorf("failed to list releases: %w", err)
		}
		if err := listResult.Err(); err != nil {
			return fmt.Errorf("failed to list releases: %w", err)
		}
		targetRelease, err = findPreviousRelease(strings.Fields(listResult.Stdout), currentRelease)
		if err != nil {
			return err
		}
	}

	// Verify target release exists
	releasePath := constants.AppReleasePath(appName, targetRelease)
	existsResult, err := client.Exec(ctx, fmt.Sprintf("test -d %s && echo 'exists'", releasePath))
	if err != nil || existsResult == nil || !strings.Contains(existsResult.Stdout, "exists") {
		available := ""
		if listResult, listErr := client.Exec(ctx, fmt.Sprintf("ls -1t %s/releases", appPath)); listErr == nil && listResult != nil {
			available = listResult.Stdout
		}
		return fmt.Errorf("release '%s' not found. Available releases:\n%s", targetRelease, available)
	}

	// Verify the release image still exists on the server
	imageName := fmt.Sprintf("%s:%s", appName, targetRelease)
	imageResult, err := client.Exec(ctx, fmt.Sprintf("docker image inspect %s --format ok 2>/dev/null", imageName))
	if err != nil || imageResult == nil || !strings.Contains(imageResult.Stdout, "ok") {
		return fmt.Errorf("image %s no longer exists on the server — cannot roll back to this release", imageName)
	}

	PrintInfo("Rolling back from %s to %s...", currentRelease, targetRelease)

	// Same pipeline as deploy: managed database URL, mounts, restart policy
	databaseURL := readSavedDatabaseURL(ctx, client, appPath)
	tempName := appName + "-rollback"

	if err := startNewContainer(ctx, client, cfg, imageName, appPath, targetRelease, databaseURL, tempName); err != nil {
		return err
	}

	// Health check the rollback container before touching the live one
	PrintInfo("Running health check...")
	if err := runHealthCheckOnContainer(ctx, client, cfg, tempName); err != nil {
		forceRemoveContainer(ctx, client, tempName)
		return fmt.Errorf("rollback aborted, current version untouched: %w", err)
	}
	PrintSuccess("Health check passed")

	// Zero-downtime swap with restore on failure (same as deploy)
	oldExists := false
	if psResult, psErr := client.Exec(ctx, fmt.Sprintf("docker ps -q -f name=^%s$", appName)); psErr == nil && psResult != nil {
		oldExists = strings.TrimSpace(psResult.Stdout) != ""
	}
	PrintInfo("Swapping containers...")
	if err := swapContainers(ctx, client, appName, appPath, targetRelease, tempName, oldExists); err != nil {
		forceRemoveContainer(ctx, client, tempName)
		return fmt.Errorf("swap failed: %w", err)
	}

	// Roll back the Messenger worker to the same image
	if cfg.Messenger.Enabled {
		PrintInfo("Rolling back Messenger worker...")
		if err := deployMessengerWorkers(ctx, client, cfg, imageName, appPath, databaseURL); err != nil {
			PrintWarning("Failed to roll back Messenger worker: %v", err)
		}
	}

	PrintSuccess("Rolled back to release %s", targetRelease)

	return nil
}
