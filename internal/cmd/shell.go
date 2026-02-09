package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
)

var shellCmd = &cobra.Command{
	Use:   "shell <server>",
	Short: "Open a shell in the application container",
	Long: `Opens an interactive shell session inside the deployed application container.

Example:
  frankendeploy shell production`,
	Args: cobra.ExactArgs(1),
	RunE: runShell,
}

var shellUser string

func init() {
	rootCmd.AddCommand(shellCmd)
	shellCmd.Flags().StringVarP(&shellUser, "user", "u", "", "User to run shell as")
}

func runShell(cmd *cobra.Command, args []string) error {
	serverName := args[0]

	// Validate user if provided
	if shellUser != "" {
		if err := security.ValidateUnixUser(shellUser); err != nil {
			return fmt.Errorf("invalid user: %w", err)
		}
	}

	conn, err := ConnectToServer(serverName)
	if err != nil {
		return err
	}
	defer conn.Client.Close()

	PrintInfo("Connecting to %s...", conn.Project.Name)

	// Build docker exec command
	execCmd := "docker exec -it"
	if shellUser != "" {
		execCmd += fmt.Sprintf(" -u %s", shellUser)
	}
	execCmd += fmt.Sprintf(" %s /bin/sh", conn.Project.Name)

	// Execute interactive shell via SSH
	return conn.Client.ExecStream(execCmd)
}
