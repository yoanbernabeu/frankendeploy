package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
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

	// Validate server name
	if err := security.ValidateServerName(serverName); err != nil {
		return fmt.Errorf("invalid server name: %w", err)
	}

	// Validate user if provided
	if shellUser != "" {
		if err := security.ValidateUnixUser(shellUser); err != nil {
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

	PrintInfo("Connecting to %s...", projectCfg.Name)

	// Connect to server
	client := ssh.NewClient(serverCfg.Host, serverCfg.User, serverCfg.Port, serverCfg.KeyPath)
	if err := client.Connect(); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer client.Close()

	// Build docker exec command
	execCmd := "docker exec -it"
	if shellUser != "" {
		execCmd += fmt.Sprintf(" -u %s", shellUser)
	}
	execCmd += fmt.Sprintf(" %s /bin/sh", projectCfg.Name)

	// Execute interactive shell via SSH
	// Note: This requires PTY support
	return client.ExecStream(execCmd)
}
