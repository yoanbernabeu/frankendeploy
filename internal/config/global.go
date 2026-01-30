package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	// GlobalConfigDir is the configuration directory name
	GlobalConfigDir = "frankendeploy"
	// GlobalConfigFile is the global config filename
	GlobalConfigFile = "config.yaml"
)

// GetGlobalConfigPath returns the path to the global config file
func GetGlobalConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		configDir = filepath.Join(homeDir, ".config")
	}

	return filepath.Join(configDir, GlobalConfigDir, GlobalConfigFile), nil
}

// LoadGlobalConfig loads the global configuration
func LoadGlobalConfig() (*GlobalConfig, error) {
	path, err := GetGlobalConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultGlobalConfig(), nil
		}
		return nil, fmt.Errorf("failed to read global config: %w", err)
	}

	var config GlobalConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse global config: %w", err)
	}

	if config.Servers == nil {
		config.Servers = make(map[string]ServerConfig)
	}

	return &config, nil
}

// SaveGlobalConfig saves the global configuration
func SaveGlobalConfig(config *GlobalConfig) error {
	path, err := GetGlobalConfigPath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	// SECURITY: Use 0700 to restrict directory access to owner only
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// SECURITY: Use 0600 to restrict file access to owner only (contains server credentials)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write global config: %w", err)
	}

	return nil
}

// GetServer retrieves a server configuration by name
func (c *GlobalConfig) GetServer(name string) (*ServerConfig, error) {
	server, ok := c.Servers[name]
	if !ok {
		return nil, fmt.Errorf("server '%s' not found", name)
	}
	return &server, nil
}

// AddServer adds a new server to the configuration
func (c *GlobalConfig) AddServer(name string, server ServerConfig) error {
	if _, exists := c.Servers[name]; exists {
		return fmt.Errorf("server '%s' already exists", name)
	}

	if server.Port == 0 {
		server.Port = c.DefaultPort
		if server.Port == 0 {
			server.Port = 22
		}
	}

	c.Servers[name] = server
	return nil
}

// RemoveServer removes a server from the configuration
func (c *GlobalConfig) RemoveServer(name string) error {
	if _, exists := c.Servers[name]; !exists {
		return fmt.Errorf("server '%s' not found", name)
	}

	delete(c.Servers, name)
	return nil
}

// ListServers returns all server names
func (c *GlobalConfig) ListServers() []string {
	names := make([]string, 0, len(c.Servers))
	for name := range c.Servers {
		names = append(names, name)
	}
	return names
}
