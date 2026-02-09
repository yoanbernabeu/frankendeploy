package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/constants"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
)

var appCmd = &cobra.Command{
	Use:   "app",
	Short: "Manage deployed applications",
	Long:  `Commands to list and manage deployed applications on servers.`,
}

var appListCmd = &cobra.Command{
	Use:   "list <server>",
	Short: "List deployed applications",
	Args:  cobra.ExactArgs(1),
	RunE:  runAppList,
}

var appRemoveCmd = &cobra.Command{
	Use:   "remove <server> <app>",
	Short: "Remove a deployed application",
	Long: `Removes a deployed application and its associated containers.

This command removes:
- Main app container
- Worker container (if exists)
- Database container (if managed)
- App directory and releases
- Caddy configuration
- Docker images

By default, all data volumes are also removed. Use --keep-data to preserve
database volumes for potential recovery or migration.

Examples:
  frankendeploy app remove production my-app --force
  frankendeploy app remove production my-app --force --keep-data`,
	Args: cobra.ExactArgs(2),
	RunE: runAppRemove,
}

var appStatusCmd = &cobra.Command{
	Use:   "status <server> [app]",
	Short: "Show application status",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runAppStatus,
}

var (
	appRemoveForce    bool
	appRemoveKeepData bool
)

func init() {
	rootCmd.AddCommand(appCmd)
	appCmd.AddCommand(appListCmd)
	appCmd.AddCommand(appRemoveCmd)
	appCmd.AddCommand(appStatusCmd)

	appRemoveCmd.Flags().BoolVarP(&appRemoveForce, "force", "f", false, "Force removal without confirmation")
	appRemoveCmd.Flags().BoolVar(&appRemoveKeepData, "keep-data", false, "Keep database volumes and persistent data")
}

func runAppList(cmd *cobra.Command, args []string) error {
	serverName := args[0]

	conn, err := ConnectToServerNoProject(serverName)
	if err != nil {
		return err
	}
	defer conn.Client.Close()

	// List apps directory
	result, err := conn.Client.Exec(fmt.Sprintf("ls -1 %s 2>/dev/null", constants.AppsDir))
	if err != nil {
		return fmt.Errorf("failed to list apps: %w", err)
	}

	apps := strings.TrimSpace(result.Stdout)
	if apps == "" {
		PrintInfo("No applications deployed on %s", serverName)
		return nil
	}

	fmt.Printf("Applications on %s:\n\n", serverName)

	for _, app := range strings.Split(apps, "\n") {
		if app == "" {
			continue
		}

		// Get container status
		statusResult, _ := conn.Client.Exec(fmt.Sprintf("docker ps --filter name=%s --format '{{.Status}}' 2>/dev/null", app))
		status := strings.TrimSpace(statusResult.Stdout)
		if status == "" {
			status = "stopped"
		}

		// Get current release
		releaseResult, _ := conn.Client.Exec(fmt.Sprintf("readlink %s/current 2>/dev/null | xargs basename", constants.AppBasePath(app)))
		release := strings.TrimSpace(releaseResult.Stdout)
		if release == "" {
			release = "-"
		}

		fmt.Printf("  %s\n", app)
		fmt.Printf("    Status:  %s\n", status)
		fmt.Printf("    Release: %s\n", release)
		fmt.Println()
	}

	return nil
}

func runAppRemove(cmd *cobra.Command, args []string) error {
	serverName := args[0]
	appName := args[1]

	// Validate app name
	if err := security.ValidateAppName(appName); err != nil {
		return fmt.Errorf("invalid app name: %w", err)
	}

	if !appRemoveForce && !IsYesMode() {
		if appRemoveKeepData {
			PrintWarning("This will remove '%s' but keep database volumes.", appName)
		} else {
			PrintWarning("This will permanently remove '%s' and ALL its data (including database).", appName)
		}
		PrintWarning("Use --force to confirm removal.")
		PrintInfo("Tip: Use --keep-data to preserve database volumes.")
		return nil
	}

	PrintInfo("Removing %s from %s...", appName, serverName)

	conn, err := ConnectToServerNoProject(serverName)
	if err != nil {
		return err
	}
	defer conn.Client.Close()

	// Stop and remove main app container
	PrintVerbose("Stopping app container...")
	if _, err := conn.Client.Exec(fmt.Sprintf("docker stop %s 2>/dev/null || true", appName)); err != nil {
		PrintVerbose("Could not stop app container: %v", err)
	}
	if _, err := conn.Client.Exec(fmt.Sprintf("docker rm %s 2>/dev/null || true", appName)); err != nil {
		PrintVerbose("Could not remove app container: %v", err)
	}

	// Stop and remove worker container if exists
	PrintVerbose("Stopping worker container...")
	if _, err := conn.Client.Exec(fmt.Sprintf("docker stop %s-worker 2>/dev/null || true", appName)); err != nil {
		PrintVerbose("Could not stop worker container: %v", err)
	}
	if _, err := conn.Client.Exec(fmt.Sprintf("docker rm %s-worker 2>/dev/null || true", appName)); err != nil {
		PrintVerbose("Could not remove worker container: %v", err)
	}

	// Stop and remove database container if exists
	PrintVerbose("Stopping database container...")
	if _, err := conn.Client.Exec(fmt.Sprintf("docker stop %s-db 2>/dev/null || true", appName)); err != nil {
		PrintVerbose("Could not stop db container: %v", err)
	}
	if _, err := conn.Client.Exec(fmt.Sprintf("docker rm %s-db 2>/dev/null || true", appName)); err != nil {
		PrintVerbose("Could not remove db container: %v", err)
	}

	// Remove volumes unless --keep-data is specified
	if !appRemoveKeepData {
		PrintVerbose("Removing data volumes...")
		if _, err := conn.Client.Exec(fmt.Sprintf("docker volume rm %s-db-data 2>/dev/null || true", appName)); err != nil {
			PrintVerbose("Could not remove db volume: %v", err)
		}
		if _, err := conn.Client.Exec(fmt.Sprintf("docker volume ls -q -f name=%s | xargs -r docker volume rm 2>/dev/null || true", appName)); err != nil {
			PrintVerbose("Could not remove app volumes: %v", err)
		}
	} else {
		PrintInfo("Keeping data volumes (use 'docker volume rm %s-db-data' to remove manually)", appName)
	}

	// Remove app directory
	result, err := conn.Client.Exec(fmt.Sprintf("rm -rf %s", constants.AppBasePath(appName)))
	if err != nil || result.ExitCode != 0 {
		return fmt.Errorf("failed to remove app directory: %s", result.Stderr)
	}

	// Remove Caddy config
	if _, err := conn.Client.Exec(fmt.Sprintf("rm -f %s", constants.CaddyAppConfig(appName))); err != nil {
		PrintVerbose("Could not remove Caddy config: %v", err)
	}

	// Reload Caddy
	if _, err := conn.Client.Exec("docker exec caddy caddy reload --config /etc/caddy/Caddyfile 2>/dev/null || true"); err != nil {
		PrintVerbose("Could not reload Caddy: %v", err)
	}

	// Remove Docker images
	if _, err := conn.Client.Exec(fmt.Sprintf("docker images %s -q | xargs -r docker rmi 2>/dev/null || true", appName)); err != nil {
		PrintVerbose("Could not remove Docker images: %v", err)
	}

	if appRemoveKeepData {
		PrintSuccess("Removed application '%s' (data volumes preserved)", appName)
	} else {
		PrintSuccess("Removed application '%s' and all its data", appName)
	}

	return nil
}

func runAppStatus(cmd *cobra.Command, args []string) error {
	serverName := args[0]

	// Determine app name
	var appName string
	if len(args) > 1 {
		appName = args[1]
		if err := security.ValidateAppName(appName); err != nil {
			return fmt.Errorf("invalid app name: %w", err)
		}
	} else {
		projectCfg, err := config.LoadProjectConfig(GetConfigFile())
		if err != nil {
			return fmt.Errorf("no app specified and no frankendeploy.yaml found")
		}
		appName = projectCfg.Name
	}

	conn, err := ConnectToServerNoProject(serverName)
	if err != nil {
		return err
	}
	defer conn.Client.Close()

	appPath := constants.AppBasePath(appName)

	fmt.Printf("Application: %s\n", appName)
	fmt.Printf("Server:      %s\n\n", serverName)

	// Container status
	result, _ := conn.Client.Exec(fmt.Sprintf("docker inspect %s --format '{{.State.Status}}' 2>/dev/null", appName))
	status := strings.TrimSpace(result.Stdout)
	if status == "" {
		status = "not deployed"
	}
	fmt.Printf("Status:      %s\n", status)

	// Current release
	result, _ = conn.Client.Exec(fmt.Sprintf("readlink %s/current 2>/dev/null | xargs basename", appPath))
	release := strings.TrimSpace(result.Stdout)
	if release != "" {
		fmt.Printf("Release:     %s\n", release)
	}

	// Uptime
	if status == "running" {
		result, _ = conn.Client.Exec(fmt.Sprintf("docker inspect %s --format '{{.State.StartedAt}}' 2>/dev/null", appName))
		startedAt := strings.TrimSpace(result.Stdout)
		if startedAt != "" {
			fmt.Printf("Started:     %s\n", startedAt)
		}
	}

	// Available releases
	result, _ = conn.Client.Exec(fmt.Sprintf("ls -1t %s/releases 2>/dev/null | head -5", appPath))
	releases := strings.TrimSpace(result.Stdout)
	if releases != "" {
		fmt.Println("\nRecent releases:")
		for _, r := range strings.Split(releases, "\n") {
			marker := "  "
			if r == release {
				marker = "* "
			}
			fmt.Printf("  %s%s\n", marker, r)
		}
	}

	return nil
}
