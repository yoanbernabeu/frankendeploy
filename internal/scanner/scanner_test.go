package scanner

import (
	"os"
	"path/filepath"
	"testing"
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
