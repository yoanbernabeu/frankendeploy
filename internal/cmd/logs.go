package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
)

var logsCmd = &cobra.Command{
	Use:   "logs <server>",
	Short: "Show application logs from server",
	Long: `Displays logs from the deployed application container.

Example:
  frankendeploy logs production
  frankendeploy logs production --tail 50
  frankendeploy logs production -f
  frankendeploy logs production --service=worker
  frankendeploy logs production --service=all`,
	Args: cobra.ExactArgs(1),
	RunE: runLogs,
}

var (
	logsFollow  bool
	logsTail    string
	logsSince   string
	logsService string
)

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().StringVar(&logsTail, "tail", "100", "Number of lines to show")
	logsCmd.Flags().StringVar(&logsSince, "since", "", "Show logs since timestamp (e.g., 2h, 30m)")
	logsCmd.Flags().StringVar(&logsService, "service", "app", "Service to show logs for (app, worker, all)")
}

func runLogs(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	serverName := args[0]

	// Validate inputs
	if err := security.ValidateLogTail(logsTail); err != nil {
		return fmt.Errorf("invalid --tail value: %w", err)
	}
	if err := security.ValidateLogSince(logsSince); err != nil {
		return fmt.Errorf("invalid --since value: %w", err)
	}

	// Validate service flag
	validServices := map[string]bool{"app": true, "worker": true, "all": true}
	if !validServices[logsService] {
		return fmt.Errorf("invalid --service value: must be 'app', 'worker', or 'all'")
	}

	conn, err := ConnectToServer(serverName)
	if err != nil {
		return err
	}
	defer conn.Client.Close()

	// Determine container name(s) based on service flag
	var containers []string
	switch logsService {
	case "app":
		containers = []string{conn.Project.Name}
	case "worker":
		containers = []string{fmt.Sprintf("%s-worker", conn.Project.Name)}
	case "all":
		containers = []string{conn.Project.Name, fmt.Sprintf("%s-worker", conn.Project.Name)}
	}

	// Show logs for each container
	for i, container := range containers {
		if len(containers) > 1 {
			PrintInfo("=== Logs for %s ===", container)
		}

		// Build docker logs command
		logsCommand := fmt.Sprintf("docker logs %s --tail %s", container, logsTail)

		if logsFollow {
			logsCommand += " -f"
		}

		if logsSince != "" {
			logsCommand += fmt.Sprintf(" --since %s", logsSince)
		}

		// Stream logs (for follow mode, only the last container will be followed)
		if logsFollow && i < len(containers)-1 {
			// For multiple containers with follow, skip follow for all but last
			logsCommand = fmt.Sprintf("docker logs %s --tail %s", container, logsTail)
			if logsSince != "" {
				logsCommand += fmt.Sprintf(" --since %s", logsSince)
			}
			result, _ := conn.Client.Exec(ctx, logsCommand)
			fmt.Print(result.Stdout)
			if result.Stderr != "" {
				fmt.Print(result.Stderr)
			}
		} else {
			if err := conn.Client.ExecStream(ctx, logsCommand); err != nil {
				// Container might not exist (e.g., no worker)
				if logsService != "all" {
					return fmt.Errorf("failed to get logs: %w", err)
				}
			}
		}
	}

	return nil
}
