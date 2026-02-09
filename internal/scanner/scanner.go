package scanner

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
)

// Scanner analyzes a Symfony project
type Scanner struct {
	projectPath string
}

// New creates a new Scanner for the given project path
func New(projectPath string) *Scanner {
	if projectPath == "" {
		projectPath = "."
	}
	return &Scanner{projectPath: projectPath}
}

// Scan performs a full project scan and returns the result
func (s *Scanner) Scan() (*config.ScanResult, error) {
	result := &config.ScanResult{}

	// Check if it's a Symfony project
	if !s.IsSymfonyProject() {
		return nil, fmt.Errorf("not a Symfony project (composer.json not found or symfony/framework-bundle missing)")
	}
	result.IsSymfony = true

	// Parse composer.json
	composer, err := s.ParseComposer()
	if err != nil {
		return nil, fmt.Errorf("failed to parse composer.json: %w", err)
	}

	result.PHPVersion = composer.PHPVersion
	result.PHPExtensions = composer.Extensions
	result.Framework = "symfony"

	// Detect database
	dbConfig, err := s.DetectDatabase()
	if err == nil {
		result.Database = *dbConfig
	}

	// Detect assets
	assetsConfig, err := s.DetectAssets()
	if err == nil {
		result.Assets = *assetsConfig
	}

	// Detect Symfony components
	result.HasDoctrine = s.HasDoctrine()
	result.HasMessenger = s.HasMessenger()
	result.HasMailer = s.HasMailer()

	// Add required extensions based on detected features
	result.PHPExtensions = s.enhanceExtensions(result.PHPExtensions, result)

	return result, nil
}

// IsSymfonyProject checks if the directory contains a Symfony project
func (s *Scanner) IsSymfonyProject() bool {
	composerPath := filepath.Join(s.projectPath, "composer.json")
	if _, err := os.Stat(composerPath); os.IsNotExist(err) {
		return false
	}

	composer, err := s.ParseComposer()
	if err != nil {
		return false
	}

	return composer.HasSymfony
}

// HasDoctrine checks if Doctrine ORM is installed
func (s *Scanner) HasDoctrine() bool {
	doctrinePath := filepath.Join(s.projectPath, "config", "packages", "doctrine.yaml")
	if _, err := os.Stat(doctrinePath); err == nil {
		return true
	}
	return false
}

// HasMessenger checks if Symfony Messenger is configured
func (s *Scanner) HasMessenger() bool {
	messengerPath := filepath.Join(s.projectPath, "config", "packages", "messenger.yaml")
	if _, err := os.Stat(messengerPath); err == nil {
		return true
	}
	return false
}

// HasMailer checks if Symfony Mailer is configured
func (s *Scanner) HasMailer() bool {
	mailerPath := filepath.Join(s.projectPath, "config", "packages", "mailer.yaml")
	if _, err := os.Stat(mailerPath); err == nil {
		return true
	}
	return false
}

// enhanceExtensions adds required PHP extensions based on detected features
func (s *Scanner) enhanceExtensions(extensions []string, result *config.ScanResult) []string {
	extMap := make(map[string]bool)
	for _, ext := range extensions {
		extMap[ext] = true
	}

	// Always include essential extensions
	essentials := []string{"intl", "opcache", "zip"}
	for _, ext := range essentials {
		if !extMap[ext] {
			extensions = append(extensions, ext)
			extMap[ext] = true
		}
	}

	// Add database-specific extension
	switch result.Database.Driver {
	case "pgsql", "pdo_pgsql":
		if !extMap["pdo_pgsql"] {
			extensions = append(extensions, "pdo_pgsql")
		}
	case "mysql", "pdo_mysql":
		if !extMap["pdo_mysql"] {
			extensions = append(extensions, "pdo_mysql")
		}
	case "sqlite", "pdo_sqlite":
		if !extMap["pdo_sqlite"] {
			extensions = append(extensions, "pdo_sqlite")
		}
	}

	// Add AMQP if Messenger is detected
	if result.HasMessenger && !extMap["amqp"] {
		extensions = append(extensions, "amqp")
	}

	return extensions
}

// ToProjectConfig converts scan result to project config
func (s *Scanner) ToProjectConfig(result *config.ScanResult, name string) *config.ProjectConfig {
	cfg := config.DefaultProjectConfig()
	cfg.Name = name
	cfg.PHP.Version = result.PHPVersion
	cfg.PHP.Extensions = result.PHPExtensions
	cfg.Database = result.Database
	cfg.Assets = result.Assets

	// For SQLite, add the database directory to shared_dirs for persistence
	if result.Database.Driver == "sqlite" && result.Database.Path != "" {
		sqliteDir := getSQLiteDirectory(result.Database.Path)
		if sqliteDir != "" && !contains(cfg.Deploy.SharedDirs, sqliteDir) {
			cfg.Deploy.SharedDirs = append(cfg.Deploy.SharedDirs, sqliteDir)
		}
	}

	// Auto-fill Messenger config if detected
	if result.HasMessenger {
		cfg.Messenger = config.MessengerConfig{
			Enabled:    true,
			Workers:    2,
			Transports: []string{"async"},
		}
	}

	// Auto-fill Mailer config if detected
	if result.HasMailer {
		cfg.Mailer = config.MailerConfig{Enabled: true}
	}

	// Auto-fill hooks based on detected features
	cfg.Deploy.Hooks = s.generateDefaultHooks(result)

	return cfg
}

// contains checks if a string slice contains a specific value
func contains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

// generateDefaultHooks creates default deployment hooks based on detected features
func (s *Scanner) generateDefaultHooks(result *config.ScanResult) config.Hooks {
	hooks := config.Hooks{}

	// If Doctrine is detected, add migration hook
	if result.HasDoctrine {
		hooks.PreDeploy = append(hooks.PreDeploy,
			"php bin/console doctrine:migrations:migrate --no-interaction --allow-no-migration")
	}

	// Always add cache warmup for Symfony
	if result.IsSymfony {
		hooks.PostDeploy = append(hooks.PostDeploy,
			"php bin/console cache:warmup")
	}

	return hooks
}
