package scanner

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SymfonyVersion returns the detected Symfony version
func (s *Scanner) SymfonyVersion() string {
	composer, err := s.ParseComposer()
	if err != nil {
		return ""
	}

	// Check for symfony/framework-bundle version
	if version := composer.GetPackageVersion("symfony/framework-bundle"); version != "" {
		return extractSymfonyVersion(version)
	}

	return ""
}

// extractSymfonyVersion extracts major.minor version from constraint
func extractSymfonyVersion(constraint string) string {
	// Remove constraint prefixes
	constraint = strings.TrimLeft(constraint, "^~>=<")

	// Take first two parts (major.minor)
	parts := strings.Split(constraint, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	if len(parts) == 1 {
		return parts[0]
	}

	return ""
}

// DoctrineConfig represents doctrine.yaml structure
type DoctrineConfig struct {
	Doctrine struct {
		DBAL struct {
			URL    string `yaml:"url"`
			Driver string `yaml:"driver"`
		} `yaml:"dbal"`
	} `yaml:"doctrine"`
}

// GetDoctrineConfig reads the doctrine.yaml configuration
func (s *Scanner) GetDoctrineConfig() (*DoctrineConfig, error) {
	doctrinePath := filepath.Join(s.projectPath, "config", "packages", "doctrine.yaml")
	data, err := os.ReadFile(doctrinePath)
	if err != nil {
		return nil, err
	}

	var config DoctrineConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// MessengerConfig represents messenger.yaml structure
type MessengerConfig struct {
	Framework struct {
		Messenger struct {
			Transports map[string]interface{} `yaml:"transports"`
		} `yaml:"messenger"`
	} `yaml:"framework"`
}

// GetMessengerConfig reads the messenger.yaml configuration
func (s *Scanner) GetMessengerConfig() (*MessengerConfig, error) {
	messengerPath := filepath.Join(s.projectPath, "config", "packages", "messenger.yaml")
	data, err := os.ReadFile(messengerPath)
	if err != nil {
		return nil, err
	}

	var config MessengerConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// GetEnvFile reads and parses a .env file
func (s *Scanner) GetEnvFile(filename string) (map[string]string, error) {
	if filename == "" {
		filename = ".env"
	}

	envPath := filepath.Join(s.projectPath, filename)
	data, err := os.ReadFile(envPath)
	if err != nil {
		return nil, err
	}

	env := make(map[string]string)
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			if (strings.HasPrefix(value, `"`) && strings.Contains(value[1:], `"`)) ||
				(strings.HasPrefix(value, `'`) && strings.Contains(value[1:], `'`)) {
				// Quoted value: extract content between quotes (preserves # inside)
				quote := value[0]
				endIdx := strings.IndexByte(value[1:], quote)
				if endIdx >= 0 {
					value = value[1 : endIdx+1]
				}
			} else {
				// Unquoted value: strip inline comment (space + #)
				if idx := strings.Index(value, " #"); idx >= 0 {
					value = strings.TrimRight(value[:idx], " ")
				}
			}

			env[key] = value
		}
	}

	return env, nil
}

// GetConfiguredBundles returns a list of configured Symfony bundles
func (s *Scanner) GetConfiguredBundles() []string {
	bundlesPath := filepath.Join(s.projectPath, "config", "bundles.php")
	data, err := os.ReadFile(bundlesPath)
	if err != nil {
		return nil
	}

	// Simple extraction of bundle class names
	content := string(data)
	var bundles []string

	// Look for patterns like "Symfony\Bundle\FrameworkBundle\FrameworkBundle::class"
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.Contains(line, "::class") {
			// Extract bundle name
			start := strings.LastIndex(line, "\\")
			end := strings.Index(line, "::class")
			if start != -1 && end != -1 && start < end {
				bundleName := line[start+1 : end]
				bundles = append(bundles, bundleName)
			}
		}
	}

	return bundles
}
