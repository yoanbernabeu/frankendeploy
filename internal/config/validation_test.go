package config

import (
	"testing"
)

// boolPtr returns a pointer to a bool value
func boolPtr(b bool) *bool {
	return &b
}

func TestValidateProjectConfig(t *testing.T) {
	tests := []struct {
		name       string
		config     *ProjectConfig
		wantErrors bool
	}{
		{
			name: "valid config",
			config: &ProjectConfig{
				Name: "my-app",
				PHP: PHPConfig{
					Version: "8.3",
				},
			},
			wantErrors: false,
		},
		{
			name: "missing name",
			config: &ProjectConfig{
				PHP: PHPConfig{
					Version: "8.3",
				},
			},
			wantErrors: true,
		},
		{
			name: "invalid name",
			config: &ProjectConfig{
				Name: "My App",
				PHP: PHPConfig{
					Version: "8.3",
				},
			},
			wantErrors: true,
		},
		{
			name: "missing PHP version",
			config: &ProjectConfig{
				Name: "my-app",
			},
			wantErrors: true,
		},
		{
			name: "invalid PHP version",
			config: &ProjectConfig{
				Name: "my-app",
				PHP: PHPConfig{
					Version: "7.4",
				},
			},
			wantErrors: true,
		},
		{
			name: "invalid database driver",
			config: &ProjectConfig{
				Name: "my-app",
				PHP: PHPConfig{
					Version: "8.3",
				},
				Database: DatabaseConfig{
					Driver: "oracle",
				},
			},
			wantErrors: true,
		},
		{
			name: "valid with database",
			config: &ProjectConfig{
				Name: "my-app",
				PHP: PHPConfig{
					Version: "8.3",
				},
				Database: DatabaseConfig{
					Driver:  "pgsql",
					Version: "16",
				},
			},
			wantErrors: false,
		},
		{
			name: "sqlite without managed is valid",
			config: &ProjectConfig{
				Name: "my-app",
				PHP: PHPConfig{
					Version: "8.3",
				},
				Database: DatabaseConfig{
					Driver: "sqlite",
					Path:   "var/data.db",
				},
			},
			wantErrors: false,
		},
		{
			name: "sqlite with managed=false is valid",
			config: &ProjectConfig{
				Name: "my-app",
				PHP: PHPConfig{
					Version: "8.3",
				},
				Database: DatabaseConfig{
					Driver:  "sqlite",
					Path:    "var/data.db",
					Managed: boolPtr(false),
				},
			},
			wantErrors: false,
		},
		{
			name: "sqlite with managed=true is invalid",
			config: &ProjectConfig{
				Name: "my-app",
				PHP: PHPConfig{
					Version: "8.3",
				},
				Database: DatabaseConfig{
					Driver:  "sqlite",
					Managed: boolPtr(true),
				},
			},
			wantErrors: true,
		},
		{
			name: "pdo_sqlite with managed=true is invalid",
			config: &ProjectConfig{
				Name: "my-app",
				PHP: PHPConfig{
					Version: "8.3",
				},
				Database: DatabaseConfig{
					Driver:  "pdo_sqlite",
					Managed: boolPtr(true),
				},
			},
			wantErrors: true,
		},
		{
			name: "pgsql with managed=true is valid",
			config: &ProjectConfig{
				Name: "my-app",
				PHP: PHPConfig{
					Version: "8.3",
				},
				Database: DatabaseConfig{
					Driver:  "pgsql",
					Version: "16",
					Managed: boolPtr(true),
				},
			},
			wantErrors: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := ValidateProjectConfig(tt.config)
			if tt.wantErrors && !errors.HasErrors() {
				t.Error("expected validation errors but got none")
			}
			if !tt.wantErrors && errors.HasErrors() {
				t.Errorf("unexpected validation errors: %s", errors.Error())
			}
		})
	}
}

func TestValidateServerConfig(t *testing.T) {
	tests := []struct {
		name       string
		config     *ServerConfig
		wantErrors bool
	}{
		{
			name: "valid config",
			config: &ServerConfig{
				Host: "example.com",
				User: "deploy",
				Port: 22,
			},
			wantErrors: false,
		},
		{
			name: "missing host",
			config: &ServerConfig{
				User: "deploy",
				Port: 22,
			},
			wantErrors: true,
		},
		{
			name: "missing user",
			config: &ServerConfig{
				Host: "example.com",
				Port: 22,
			},
			wantErrors: true,
		},
		{
			name: "invalid port",
			config: &ServerConfig{
				Host: "example.com",
				User: "deploy",
				Port: 0,
			},
			wantErrors: true,
		},
		{
			name: "port too high",
			config: &ServerConfig{
				Host: "example.com",
				User: "deploy",
				Port: 70000,
			},
			wantErrors: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := ValidateServerConfig(tt.config)
			if tt.wantErrors && !errors.HasErrors() {
				t.Error("expected validation errors but got none")
			}
			if !tt.wantErrors && errors.HasErrors() {
				t.Errorf("unexpected validation errors: %s", errors.Error())
			}
		})
	}
}
