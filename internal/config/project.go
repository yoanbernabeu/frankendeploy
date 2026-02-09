package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	config.Database.Driver = NormalizeDBDriver(config.Database.Driver)

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

	if err := os.WriteFile(path, data, 0644); err != nil {
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
