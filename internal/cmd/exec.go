package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
)

var execCmd = &cobra.Command{
	Use:   "exec <server> <command>",
	Short: "Execute a command in the application container",
	Long: `Executes a command inside the deployed application container.

Example:
  frankendeploy exec production php bin/console cache:clear
  frankendeploy exec production composer install`,
	Args: cobra.MinimumNArgs(2),
	RunE: runExec,
}

var execUser string

func init() {
	rootCmd.AddCommand(execCmd)
	execCmd.Flags().StringVarP(&execUser, "user", "u", "", "User to run command as")
}

func runExec(cmd *cobra.Command, args []string) error {
	serverName := args[0]
	command := strings.Join(args[1:], " ")

	// Validate command for shell injection
	if err := security.ValidateDockerCommand(command); err != nil {
		return fmt.Errorf("invalid command: %w", err)
	}

	// Validate user if provided
	if execUser != "" {
		if err := security.ValidateUnixUser(execUser); err != nil {
			return fmt.Errorf("invalid user: %w", err)
		}
	}

	conn, err := ConnectToServer(serverName)
	if err != nil {
		return err
	}
	defer conn.Client.Close()

	// Build docker exec command
	dockerExec := "docker exec"
	if execUser != "" {
		dockerExec += fmt.Sprintf(" -u %s", execUser)
	}
	dockerExec += fmt.Sprintf(" %s %s", conn.Project.Name, command)

	// Execute and stream output
	return conn.Client.ExecStream(dockerExec)
}
