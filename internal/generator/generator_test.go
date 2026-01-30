package generator

import (
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
)

func TestDockerfileGenerator_Generate(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP: config.PHPConfig{
			Version:    "8.3",
			Extensions: []string{"pdo_pgsql", "intl", "opcache"},
		},
		Deploy: config.DeployConfig{
			HealthcheckPath: "/health",
		},
	}

	gen := NewDockerfileGenerator(cfg)
	dockerfile, err := gen.Generate()

	if err != nil {
		t.Fatalf("failed to generate Dockerfile: %v", err)
	}

	// Check key elements
	if !strings.Contains(dockerfile, "dunglas/frankenphp") {
		t.Error("Dockerfile should use FrankenPHP base image")
	}

	if !strings.Contains(dockerfile, "php8.3") {
		t.Error("Dockerfile should specify PHP 8.3")
	}

	if !strings.Contains(dockerfile, "pdo_pgsql") {
		t.Error("Dockerfile should include pdo_pgsql extension")
	}

	if !strings.Contains(dockerfile, "HEALTHCHECK") {
		t.Error("Dockerfile should include health check")
	}
}

func TestDockerfileGenerator_GenerateDockerignore(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP: config.PHPConfig{
			Version: "8.3",
		},
	}

	gen := NewDockerfileGenerator(cfg)
	dockerignore := gen.GenerateDockerignore()

	expectedPatterns := []string{
		".git",
		"vendor",
		"node_modules",
		"var/cache",
		"Dockerfile",
		"compose",
		".env.local",
	}

	for _, pattern := range expectedPatterns {
		if !strings.Contains(dockerignore, pattern) {
			t.Errorf("dockerignore should contain %q", pattern)
		}
	}
}

func TestComposeGenerator_GenerateDev(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP: config.PHPConfig{
			Version: "8.3",
		},
		Database: config.DatabaseConfig{
			Driver:  "pgsql",
			Version: "16",
		},
	}

	gen := NewComposeGenerator(cfg)
	compose, err := gen.GenerateDev()

	if err != nil {
		t.Fatalf("failed to generate docker-compose: %v", err)
	}

	if !strings.Contains(compose, "test-app") {
		t.Error("compose should contain app name")
	}

	if !strings.Contains(compose, "postgres") {
		t.Error("compose should include PostgreSQL for pgsql driver")
	}

	if !strings.Contains(compose, "8000:8080") {
		t.Error("compose should expose port 8000 mapped to internal port 8080")
	}
}

func TestComposeGenerator_BuildDatabaseURL(t *testing.T) {
	tests := []struct {
		driver   string
		expected string
	}{
		{"pgsql", "postgresql://"},
		{"mysql", "mysql://"},
		{"sqlite", "sqlite://"},
	}

	for _, tt := range tests {
		t.Run(tt.driver, func(t *testing.T) {
			cfg := &config.ProjectConfig{
				Database: config.DatabaseConfig{
					Driver:  tt.driver,
					Version: "16",
				},
			}

			gen := NewComposeGenerator(cfg)
			url := gen.buildDatabaseURL()

			if !strings.HasPrefix(url, tt.expected) {
				t.Errorf("buildDatabaseURL() for %s should start with %q, got %q", tt.driver, tt.expected, url)
			}
		})
	}
}
