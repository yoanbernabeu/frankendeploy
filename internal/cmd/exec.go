package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
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

	// Validate server name
	if err := security.ValidateServerName(serverName); err != nil {
		return fmt.Errorf("invalid server name: %w", err)
	}

	// Validate user if provided
	if execUser != "" {
		if err := security.ValidateUnixUser(execUser); err != nil {
			return fmt.Errorf("invalid user: %w", err)
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

	// Connect to server
	client := ssh.NewClient(serverCfg.Host, serverCfg.User, serverCfg.Port, serverCfg.KeyPath)
	if err := client.Connect(); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer client.Close()

	// Build docker exec command
	dockerExec := "docker exec"
	if execUser != "" {
		dockerExec += fmt.Sprintf(" -u %s", execUser)
	}
	dockerExec += fmt.Sprintf(" %s %s", projectCfg.Name, command)

	// Execute and stream output
	return client.ExecStream(dockerExec)
}
