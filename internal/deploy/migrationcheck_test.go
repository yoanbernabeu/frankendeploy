package deploy

import (
	"strings"
	"testing"
)

func TestMigrationCheckResult_HasPotentialProblem(t *testing.T) {
	tests := []struct {
		name            string
		migrationFiles  int
		entityFiles     int
		expectedProblem bool
	}{
		{
			name:            "no entities, no migrations - no problem",
			migrationFiles:  0,
			entityFiles:     0,
			expectedProblem: false,
		},
		{
			name:            "entities exist, no migrations - problem",
			migrationFiles:  0,
			entityFiles:     5,
			expectedProblem: true,
		},
		{
			name:            "entities exist, migrations exist - no problem",
			migrationFiles:  3,
			entityFiles:     5,
			expectedProblem: false,
		},
		{
			name:            "no entities, migrations exist - no problem",
			migrationFiles:  3,
			entityFiles:     0,
			expectedProblem: false,
		},
		{
			name:            "single entity, no migrations - problem",
			migrationFiles:  0,
			entityFiles:     1,
			expectedProblem: true,
		},
		{
			name:            "single entity, single migration - no problem",
			migrationFiles:  1,
			entityFiles:     1,
			expectedProblem: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &MigrationCheckResult{
				MigrationFilesCount: tt.migrationFiles,
				EntityFilesCount:    tt.entityFiles,
				HasPotentialProblem: tt.entityFiles > 0 && tt.migrationFiles == 0,
			}

			if result.HasPotentialProblem != tt.expectedProblem {
				t.Errorf("HasPotentialProblem = %v, expected %v", result.HasPotentialProblem, tt.expectedProblem)
			}
		})
	}
}

func TestFormatMigrationWarning(t *testing.T) {
	result := &MigrationCheckResult{
		MigrationFilesCount: 0,
		EntityFilesCount:    5,
		HasPotentialProblem: true,
	}

	warning := FormatMigrationWarning(result)

	expectedStrings := []string{
		"No database migrations found but entities exist",
		"5 files in src/Entity/",
		"0 files in migrations/",
		"no such table",
		"make:migration",
		"doctrine:migrations:migrate",
		"git add migrations/",
		"git commit",
		"redeploy",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(warning, expected) {
			t.Errorf("FormatMigrationWarning() missing expected string: %s", expected)
		}
	}
}

func TestHasMigrationHook(t *testing.T) {
	tests := []struct {
		name     string
		hooks    []string
		expected bool
	}{
		{
			name:     "empty hooks",
			hooks:    []string{},
			expected: false,
		},
		{
			name:     "no migration hook",
			hooks:    []string{"php bin/console cache:clear"},
			expected: false,
		},
		{
			name:     "full migration command",
			hooks:    []string{"php bin/console doctrine:migrations:migrate --no-interaction"},
			expected: true,
		},
		{
			name:     "short migration command",
			hooks:    []string{"php bin/console d:m:m --no-interaction"},
			expected: true,
		},
		{
			name:     "migration among other hooks",
			hooks:    []string{"php bin/console cache:clear", "php bin/console doctrine:migrations:migrate --no-interaction", "php bin/console assets:install"},
			expected: true,
		},
		{
			name:     "messenger stop workers (not migration)",
			hooks:    []string{"php bin/console messenger:stop-workers"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasMigrationHook(tt.hooks)
			if result != tt.expected {
				t.Errorf("HasMigrationHook() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestParseCount(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "simple number",
			input:    "5",
			expected: 5,
		},
		{
			name:     "number with whitespace",
			input:    "  10  \n",
			expected: 10,
		},
		{
			name:     "zero",
			input:    "0",
			expected: 0,
		},
		{
			name:     "empty string",
			input:    "",
			expected: 0,
		},
		{
			name:     "invalid input",
			input:    "not a number",
			expected: 0,
		},
		{
			name:     "newline only",
			input:    "\n",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCount(tt.input)
			if result != tt.expected {
				t.Errorf("parseCount(%q) = %d, expected %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetMigrationWarningMarkerPath(t *testing.T) {
	path := getMigrationWarningMarkerPath("my-app")
	expected := "/opt/frankendeploy/apps/my-app/shared/.migration_warning_shown"

	if path != expected {
		t.Errorf("getMigrationWarningMarkerPath() = %s, expected %s", path, expected)
	}
}
