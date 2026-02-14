package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
)

func TestScanner_IsSymfonyProject(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Test without composer.json
	s := New(tempDir)
	if s.IsSymfonyProject() {
		t.Error("expected false for non-Symfony project")
	}

	// Create a basic composer.json with Symfony
	composerContent := `{
		"require": {
			"php": ">=8.1",
			"symfony/framework-bundle": "^6.4"
		}
	}`
	err := os.WriteFile(filepath.Join(tempDir, "composer.json"), []byte(composerContent), 0644)
	if err != nil {
		t.Fatalf("failed to create composer.json: %v", err)
	}

	// Test with Symfony project
	s = New(tempDir)
	if !s.IsSymfonyProject() {
		t.Error("expected true for Symfony project")
	}
}

func TestScanner_DetectPackageManager(t *testing.T) {
	tempDir := t.TempDir()
	s := New(tempDir)

	// Default should be npm
	if s.detectPackageManager() != "npm" {
		t.Error("expected npm as default package manager")
	}

	// Create yarn.lock
	err := os.WriteFile(filepath.Join(tempDir, "yarn.lock"), []byte{}, 0644)
	if err != nil {
		t.Fatalf("failed to create yarn.lock: %v", err)
	}

	if s.detectPackageManager() != "yarn" {
		t.Error("expected yarn when yarn.lock exists")
	}

	// Create pnpm-lock.yaml (should take precedence)
	err = os.WriteFile(filepath.Join(tempDir, "pnpm-lock.yaml"), []byte{}, 0644)
	if err != nil {
		t.Fatalf("failed to create pnpm-lock.yaml: %v", err)
	}

	if s.detectPackageManager() != "pnpm" {
		t.Error("expected pnpm when pnpm-lock.yaml exists")
	}
}

func TestEnhanceExtensions_Dedup(t *testing.T) {
	s := New(t.TempDir())
	result := &config.ScanResult{
		Database: config.DatabaseConfig{Driver: "pgsql"},
	}

	// Input with duplicates: pdo_pgsql appears in composer AND would be added by enhanceExtensions
	input := []string{"intl", "opcache", "zip", "pdo_pgsql", "intl", "zip"}
	got := s.enhanceExtensions(input, result)

	// Check no duplicates
	seen := make(map[string]bool)
	for _, ext := range got {
		if seen[ext] {
			t.Errorf("duplicate extension found: %q", ext)
		}
		seen[ext] = true
	}

	// Ensure pdo_pgsql is present
	if !seen["pdo_pgsql"] {
		t.Error("expected pdo_pgsql in result")
	}
}

func TestExtractPHPVersion(t *testing.T) {
	tests := []struct {
		constraint string
		expected   string
	}{
		{">=8.1", "8.1"},
		{"^8.2", "8.2"},
		{"~8.3", "8.3"},
		{"8.2.*", "8.2"},
		{">=8.1 <8.4", "8.4"},
		{"^8.5", "8.5"},
		{">=8.10", "8.10"},
		{">=7.4", "8.3"}, // No 8.x found, should default
		{"", "8.3"},
	}

	for _, tt := range tests {
		t.Run(tt.constraint, func(t *testing.T) {
			result := extractPHPVersion(tt.constraint)
			if result != tt.expected {
				t.Errorf("extractPHPVersion(%q) = %q, want %q", tt.constraint, result, tt.expected)
			}
		})
	}
}

func TestGetEnvFile_InlineComments(t *testing.T) {
	tempDir := t.TempDir()

	envContent := `# Full line comment
APP_ENV=prod
APP_SECRET=abc123 # this is a comment
DATABASE_URL="postgresql://user:pass#word@localhost/db" # connection string
QUOTED_HASH='value#with#hashes'
NO_SPACE_HASH=value#notacomment
EMPTY_VALUE=
`
	if err := os.WriteFile(filepath.Join(tempDir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	s := New(tempDir)
	env, err := s.GetEnvFile(".env")
	if err != nil {
		t.Fatalf("GetEnvFile() error = %v", err)
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"APP_ENV", "prod"},
		{"APP_SECRET", "abc123"},
		{"DATABASE_URL", "postgresql://user:pass#word@localhost/db"},
		{"QUOTED_HASH", "value#with#hashes"},
		{"NO_SPACE_HASH", "value#notacomment"},
		{"EMPTY_VALUE", ""},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, ok := env[tt.key]
			if !ok {
				t.Fatalf("key %q not found in env", tt.key)
			}
			if got != tt.expected {
				t.Errorf("env[%q] = %q, want %q", tt.key, got, tt.expected)
			}
		})
	}
}
