package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yoanbernabeu/frankendeploy/internal/security"
	"gopkg.in/yaml.v3"
)

const (
	// ProjectConfigFile is the default project config filename
	ProjectConfigFile = "frankendeploy.yaml"
)

// LoadProjectConfig loads the project configuration from the given path
func LoadProjectConfig(path string) (*ProjectConfig, error) {
	if path == "" {
		path = ProjectConfigFile
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("configuration file not found: %s (run 'frankendeploy init' first)", path)
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config ProjectConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse config file (check for typos or unknown fields): %w", err)
	}

	config.Database.Driver = NormalizeDBDriver(config.Database.Driver)

	// Validate app name
	if config.Name != "" {
		if err := security.ValidateAppName(config.Name); err != nil {
			return nil, fmt.Errorf("invalid app name in config: %w", err)
		}
	}

	// Validate deployment hooks
	for _, hook := range config.Deploy.Hooks.PreDeploy {
		if err := security.ValidateHook(hook); err != nil {
			return nil, fmt.Errorf("invalid pre_deploy hook %q: %w", hook, err)
		}
	}
	for _, hook := range config.Deploy.Hooks.PostDeploy {
		if err := security.ValidateHook(hook); err != nil {
			return nil, fmt.Errorf("invalid post_deploy hook %q: %w", hook, err)
		}
	}

	// Validate shared directories
	for _, dir := range config.Deploy.SharedDirs {
		if err := security.ValidateSharedDir(dir); err != nil {
			return nil, fmt.Errorf("invalid shared_dir %q: %w", dir, err)
		}
	}

	return &config, nil
}

// SaveProjectConfig saves the project configuration to the given path
func SaveProjectConfig(config *ProjectConfig, path string) error {
	if path == "" {
		path = ProjectConfigFile
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// ProjectConfigExists checks if the project config file exists
func ProjectConfigExists(path string) bool {
	if path == "" {
		path = ProjectConfigFile
	}
	_, err := os.Stat(path)
	return err == nil
}

// FindProjectConfig searches for the config file in current and parent directories
func FindProjectConfig() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	dir := cwd
	for {
		configPath := filepath.Join(dir, ProjectConfigFile)
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("no %s found in current or parent directories", ProjectConfigFile)
}

// NormalizeDBDriver normalizes database driver names to their canonical form.
// For example, "pdo_pgsql" → "pgsql", "pdo_mysql" → "mysql", "pdo_sqlite" → "sqlite".
func NormalizeDBDriver(driver string) string {
	switch strings.ToLower(driver) {
	case "pdo_pgsql", "postgresql", "postgres":
		return "pgsql"
	case "pdo_mysql", "mysqli":
		return "mysql"
	case "pdo_sqlite", "sqlite3":
		return "sqlite"
	default:
		return driver
	}
}
