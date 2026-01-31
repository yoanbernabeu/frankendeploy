package scanner

import (
	"testing"
)

func TestExtractSQLitePath(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "simple sqlite URL",
			url:      "sqlite:///var/data.db",
			expected: "var/data.db",
		},
		{
			name:     "sqlite URL with kernel.project_dir",
			url:      "sqlite:///%kernel.project_dir%/var/data.db",
			expected: "var/data.db",
		},
		{
			name:     "sqlite URL with nested path",
			url:      "sqlite:///var/database/app.db",
			expected: "var/database/app.db",
		},
		{
			name:     "empty path defaults to var/data.db",
			url:      "sqlite://",
			expected: "var/data.db",
		},
		{
			name:     "sqlite: prefix without double slash",
			url:      "sqlite:/var/data.db",
			expected: "var/data.db",
		},
		{
			name:     "uppercase SQLITE URL",
			url:      "SQLITE:///var/data.db",
			expected: "var/data.db",
		},
		{
			name:     "mixed case sqlite URL",
			url:      "SQLite:///var/DATA.db",
			expected: "var/DATA.db",
		},
		{
			name:     "path with multiple kernel.project_dir",
			url:      "sqlite:///%kernel.project_dir%/var/%kernel.project_dir%/data.db",
			expected: "var//data.db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSQLitePath(tt.url)
			if result != tt.expected {
				t.Errorf("extractSQLitePath(%q) = %q, want %q", tt.url, result, tt.expected)
			}
		})
	}
}

func TestGetSQLiteDirectory(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "path with directory",
			path:     "var/data.db",
			expected: "var",
		},
		{
			name:     "nested path",
			path:     "var/database/app.db",
			expected: "var/database",
		},
		{
			name:     "file in root",
			path:     "data.db",
			expected: "",
		},
		{
			name:     "empty path",
			path:     "",
			expected: "",
		},
		{
			name:     "deep nested path",
			path:     "var/lib/sqlite/data/app.db",
			expected: "var/lib/sqlite/data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSQLiteDirectory(tt.path)
			if result != tt.expected {
				t.Errorf("getSQLiteDirectory(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestParseDBURL_SQLite(t *testing.T) {
	tests := []struct {
		name           string
		url            string
		expectedDriver string
		expectedVer    string
	}{
		{
			name:           "sqlite URL",
			url:            "sqlite:///var/data.db",
			expectedDriver: "sqlite",
			expectedVer:    "3",
		},
		{
			name:           "postgresql URL",
			url:            "postgresql://user:pass@localhost:5432/db",
			expectedDriver: "pgsql",
			expectedVer:    "16",
		},
		{
			name:           "mysql URL",
			url:            "mysql://user:pass@localhost:3306/db",
			expectedDriver: "mysql",
			expectedVer:    "8.0",
		},
		{
			name:           "postgres URL alias",
			url:            "postgres://user:pass@localhost:5432/db",
			expectedDriver: "pgsql",
			expectedVer:    "16",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver, version := parseDBURL(tt.url)
			if driver != tt.expectedDriver {
				t.Errorf("parseDBURL(%q) driver = %q, want %q", tt.url, driver, tt.expectedDriver)
			}
			if version != tt.expectedVer {
				t.Errorf("parseDBURL(%q) version = %q, want %q", tt.url, version, tt.expectedVer)
			}
		})
	}
}

func TestNormalizeDriver(t *testing.T) {
	tests := []struct {
		driver   string
		expected string
	}{
		{"pdo_pgsql", "pgsql"},
		{"postgresql", "pgsql"},
		{"postgres", "pgsql"},
		{"pgsql", "pgsql"},
		{"pdo_mysql", "mysql"},
		{"mysql", "mysql"},
		{"mysqli", "mysql"},
		{"pdo_sqlite", "sqlite"},
		{"sqlite", "sqlite"},
		{"sqlite3", "sqlite"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.driver, func(t *testing.T) {
			result := normalizeDriver(tt.driver)
			if result != tt.expected {
				t.Errorf("normalizeDriver(%q) = %q, want %q", tt.driver, result, tt.expected)
			}
		})
	}
}
