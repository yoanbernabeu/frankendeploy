package cmd

import (
	"fmt"
	"time"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

// ServerConnection holds a connected SSH client along with project and server config.
type ServerConnection struct {
	Client  *ssh.Client
	Project *config.ProjectConfig
	Server  *config.ServerConfig
	Global  *config.GlobalConfig
}

// ConnectToServer validates the server name, loads project + global config,
// and establishes an SSH connection. The caller must defer conn.Client.Close().
func ConnectToServer(serverName string, opts ...ssh.ClientOption) (*ServerConnection, error) {
	if err := security.ValidateServerName(serverName); err != nil {
		return nil, fmt.Errorf("invalid server name: %w", err)
	}

	projectCfg, err := config.LoadProjectConfig(GetConfigFile())
	if err != nil {
		return nil, fmt.Errorf("failed to load project config: %w", err)
	}

	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load global config: %w", err)
	}

	serverCfg, err := globalCfg.GetServer(serverName)
	if err != nil {
		return nil, err
	}

	allOpts := sshOptsFromGlobal(globalCfg, opts)

	client := ssh.NewClient(serverCfg.Host, serverCfg.User, serverCfg.Port, serverCfg.KeyPath, allOpts...)
	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	return &ServerConnection{
		Client:  client,
		Project: projectCfg,
		Server:  serverCfg,
		Global:  globalCfg,
	}, nil
}

// ConnectToServerNoProject validates the server name, loads global config only,
// and establishes an SSH connection. Used for commands that don't require a project config
// (e.g., server setup, app list). The caller must defer conn.Client.Close().
func ConnectToServerNoProject(serverName string, opts ...ssh.ClientOption) (*ServerConnection, error) {
	if err := security.ValidateServerName(serverName); err != nil {
		return nil, fmt.Errorf("invalid server name: %w", err)
	}

	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load global config: %w", err)
	}

	serverCfg, err := globalCfg.GetServer(serverName)
	if err != nil {
		return nil, err
	}

	allOpts := sshOptsFromGlobal(globalCfg, opts)

	client := ssh.NewClient(serverCfg.Host, serverCfg.User, serverCfg.Port, serverCfg.KeyPath, allOpts...)
	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	return &ServerConnection{
		Client:  client,
		Project: nil,
		Server:  serverCfg,
		Global:  globalCfg,
	}, nil
}

// sshOptsFromGlobal prepends a WithTimeout option if SSHTimeout is configured.
func sshOptsFromGlobal(globalCfg *config.GlobalConfig, opts []ssh.ClientOption) []ssh.ClientOption {
	if globalCfg.SSHTimeout > 0 {
		timeoutOpt := ssh.WithTimeout(time.Duration(globalCfg.SSHTimeout) * time.Second)
		return append([]ssh.ClientOption{timeoutOpt}, opts...)
	}
	return opts
}
