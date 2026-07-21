package security

// Tests for issue #56: single sensitive-key list shared by log masking
// and display masking.

import (
	"strings"
	"testing"
)

func TestIsSensitiveEnvKey(t *testing.T) {
	sensitive := []string{
		"APP_SECRET", "DATABASE_URL", "MAILER_DSN", "MY_API_KEY",
		"GITHUB_TOKEN", "POSTGRES_PASSWORD", "MYSQL_ROOT_PASSWORD",
		"SMTP_PASS", "JWT_SECRET_KEY", "SENTRY_DSN", "app_secret",
	}
	for _, key := range sensitive {
		if !IsSensitiveEnvKey(key) {
			t.Errorf("IsSensitiveEnvKey(%q) = false, want true", key)
		}
	}

	notSensitive := []string{"APP_ENV", "APP_DEBUG", "TZ", "LOCALE", "TRUSTED_PROXIES"}
	for _, key := range notSensitive {
		if IsSensitiveEnvKey(key) {
			t.Errorf("IsSensitiveEnvKey(%q) = true, want false", key)
		}
	}
}

func TestSanitizeCommandForLog_MasksAllSensitiveKeys(t *testing.T) {
	tests := []struct {
		name   string
		cmd    string
		hidden string
		kept   string
	}{
		{
			name:   "DATABASE_URL",
			cmd:    `docker run -e DATABASE_URL=postgresql://user:hunter2@host/db app`,
			hidden: "hunter2",
			kept:   "docker run",
		},
		{
			name:   "MAILER_DSN",
			cmd:    `docker run -e MAILER_DSN=smtp://user:hunter2@smtp.example.com app`,
			hidden: "hunter2",
			kept:   "docker run",
		},
		{
			name:   "custom token",
			cmd:    `export GITHUB_TOKEN=ghp_supersecret123`,
			hidden: "ghp_supersecret123",
			kept:   "export",
		},
		{
			name:   "custom api key",
			cmd:    `run -e STRIPE_API_KEY="sk_live_secret"`,
			hidden: "sk_live_secret",
			kept:   "run",
		},
		{
			name:   "app secret",
			cmd:    `echo APP_SECRET=abc123 >> .env.local`,
			hidden: "abc123",
			kept:   ".env.local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeCommandForLog(tt.cmd)
			if strings.Contains(got, tt.hidden) {
				t.Errorf("secret %q leaked in sanitized output: %q", tt.hidden, got)
			}
			if !strings.Contains(got, tt.kept) {
				t.Errorf("non-secret part %q lost in sanitized output: %q", tt.kept, got)
			}
		})
	}
}

func TestSanitizeCommandForLog_KeepsNonSensitiveValues(t *testing.T) {
	cmd := `docker run -e APP_ENV=prod -e APP_DEBUG=0 app`
	got := SanitizeCommandForLog(cmd)
	if !strings.Contains(got, "APP_ENV=prod") || !strings.Contains(got, "APP_DEBUG=0") {
		t.Errorf("non-sensitive values must not be masked: %q", got)
	}
}

func TestSanitizeCommandForLog_MySQLPasswordFlag(t *testing.T) {
	got := SanitizeCommandForLog(`mysqldump -phunter2 -u root db`)
	if strings.Contains(got, "hunter2") {
		t.Errorf("MySQL -p password leaked: %q", got)
	}
}
