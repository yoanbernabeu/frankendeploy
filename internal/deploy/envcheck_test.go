package deploy

import (
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
)

func TestNeedsDatabaseURL(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.ProjectConfig
		expected bool
	}{
		{
			name: "no database configured",
			cfg: &config.ProjectConfig{
				Database: config.DatabaseConfig{},
			},
			expected: false,
		},
		{
			name: "managed database",
			cfg: &config.ProjectConfig{
				Database: config.DatabaseConfig{
					Driver:  "pgsql",
					Managed: boolPtr(true),
				},
			},
			expected: false,
		},
		{
			name: "sqlite database",
			cfg: &config.ProjectConfig{
				Database: config.DatabaseConfig{
					Driver: "sqlite",
				},
			},
			expected: false,
		},
		{
			name: "pdo_sqlite database",
			cfg: &config.ProjectConfig{
				Database: config.DatabaseConfig{
					Driver: "pdo_sqlite",
				},
			},
			expected: false,
		},
		{
			name: "external pgsql database",
			cfg: &config.ProjectConfig{
				Database: config.DatabaseConfig{
					Driver:  "pgsql",
					Managed: boolPtr(false),
				},
			},
			expected: true,
		},
		{
			name: "external mysql database",
			cfg: &config.ProjectConfig{
				Database: config.DatabaseConfig{
					Driver:  "mysql",
					Managed: boolPtr(false),
				},
			},
			expected: true,
		},
		{
			name: "database with no managed flag (defaults to external)",
			cfg: &config.ProjectConfig{
				Database: config.DatabaseConfig{
					Driver: "pgsql",
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := needsDatabaseURL(tt.cfg)
			if result != tt.expected {
				t.Errorf("needsDatabaseURL() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGenerateSymfonySecret(t *testing.T) {
	secret, err := GenerateSymfonySecret()
	if err != nil {
		t.Fatalf("GenerateSymfonySecret() error = %v", err)
	}

	// Check length (64 hex characters = 32 bytes)
	if len(secret) != 64 {
		t.Errorf("GenerateSymfonySecret() length = %d, expected 64", len(secret))
	}

	// Check it's valid hex
	for _, c := range secret {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("GenerateSymfonySecret() contains invalid hex character: %c", c)
		}
	}

	// Check uniqueness (generate another and compare)
	secret2, err := GenerateSymfonySecret()
	if err != nil {
		t.Fatalf("GenerateSymfonySecret() second call error = %v", err)
	}
	if secret == secret2 {
		t.Error("GenerateSymfonySecret() generated same secret twice")
	}
}

func TestGenerateMissingSecrets(t *testing.T) {
	tests := []struct {
		name        string
		missing     []EnvRequirement
		wantKeys    []string
		wantEmpty   bool
	}{
		{
			name:      "empty missing list",
			missing:   []EnvRequirement{},
			wantKeys:  []string{},
			wantEmpty: true,
		},
		{
			name: "APP_SECRET can be generated",
			missing: []EnvRequirement{
				{Name: "APP_SECRET", CanGenerate: true},
			},
			wantKeys:  []string{"APP_SECRET"},
			wantEmpty: false,
		},
		{
			name: "DATABASE_URL cannot be generated",
			missing: []EnvRequirement{
				{Name: "DATABASE_URL", CanGenerate: false},
			},
			wantKeys:  []string{},
			wantEmpty: true,
		},
		{
			name: "mixed requirements",
			missing: []EnvRequirement{
				{Name: "APP_SECRET", CanGenerate: true},
				{Name: "DATABASE_URL", CanGenerate: false},
			},
			wantKeys:  []string{"APP_SECRET"},
			wantEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generated, err := GenerateMissingSecrets(tt.missing)
			if err != nil {
				t.Fatalf("GenerateMissingSecrets() error = %v", err)
			}

			if tt.wantEmpty && len(generated) > 0 {
				t.Errorf("GenerateMissingSecrets() = %v, expected empty map", generated)
			}

			for _, key := range tt.wantKeys {
				if _, exists := generated[key]; !exists {
					t.Errorf("GenerateMissingSecrets() missing key %s", key)
				}
			}
		})
	}
}

func TestFormatEnvCheckError(t *testing.T) {
	missing := []EnvRequirement{
		{Name: "APP_SECRET", Description: "Symfony security secret"},
		{Name: "DATABASE_URL", Description: "Database connection URL"},
	}

	result := FormatEnvCheckError(missing, "prod")

	// Check that all required elements are present
	expectedStrings := []string{
		"Missing required environment variables",
		"APP_SECRET",
		"DATABASE_URL",
		"frankendeploy env set prod APP_SECRET",
		"frankendeploy env set prod DATABASE_URL",
		"--force",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("FormatEnvCheckError() missing expected string: %s", expected)
		}
	}
}

func TestParseEnvContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected map[string]string
	}{
		{
			name:     "empty content",
			content:  "",
			expected: map[string]string{},
		},
		{
			name:    "simple key=value",
			content: "APP_ENV=prod",
			expected: map[string]string{
				"APP_ENV": "prod",
			},
		},
		{
			name:    "quoted value",
			content: `DATABASE_URL="postgresql://user:pass@host/db"`,
			expected: map[string]string{
				"DATABASE_URL": "postgresql://user:pass@host/db",
			},
		},
		{
			name:    "multiple variables",
			content: "APP_ENV=prod\nAPP_SECRET=mysecret",
			expected: map[string]string{
				"APP_ENV":    "prod",
				"APP_SECRET": "mysecret",
			},
		},
		{
			name:    "with comments",
			content: "# This is a comment\nAPP_ENV=prod",
			expected: map[string]string{
				"APP_ENV": "prod",
			},
		},
		{
			name:    "with empty lines",
			content: "APP_ENV=prod\n\nAPP_SECRET=secret",
			expected: map[string]string{
				"APP_ENV":    "prod",
				"APP_SECRET": "secret",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseEnvContent(tt.content)
			if len(result) != len(tt.expected) {
				t.Errorf("parseEnvContent() returned %d items, expected %d", len(result), len(tt.expected))
			}
			for key, expectedValue := range tt.expected {
				if result[key] != expectedValue {
					t.Errorf("parseEnvContent()[%s] = %s, expected %s", key, result[key], expectedValue)
				}
			}
		})
	}
}

func TestBuildEnvContent(t *testing.T) {
	vars := map[string]string{
		"APP_ENV": "prod",
	}

	result := buildEnvContent(vars)

	if !strings.Contains(result, "APP_ENV=prod") {
		t.Errorf("buildEnvContent() = %s, expected to contain APP_ENV=prod", result)
	}

	// Check it ends with newline
	if !strings.HasSuffix(result, "\n") {
		t.Error("buildEnvContent() should end with newline")
	}
}

// Helper function for bool pointers
func boolPtr(b bool) *bool {
	return &b
}
