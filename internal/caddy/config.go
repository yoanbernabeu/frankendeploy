package caddy

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
)

// ConfigGenerator generates Caddy configuration files
type ConfigGenerator struct{}

// NewConfigGenerator creates a new Caddy config generator
func NewConfigGenerator() *ConfigGenerator {
	return &ConfigGenerator{}
}

// AppConfig represents configuration for a single app
type AppConfig struct {
	Name   string
	Domain string
	Port   int
	TLS    bool
}

// GenerateAppConfig generates Caddy config for an application
func (g *ConfigGenerator) GenerateAppConfig(app AppConfig) (string, error) {
	tmpl := `# {{ .Name }}
{{ .Domain }} {
{{- if not .TLS }}
    tls internal
{{- end }}

    reverse_proxy {{ .Name }}:{{ .Port }} {
        health_uri /
        health_interval 30s
        health_timeout 5s
    }

    encode zstd gzip

    header {
        X-Content-Type-Options nosniff
        X-Frame-Options DENY
        Referrer-Policy strict-origin-when-cross-origin
        -Server
    }

    log {
        output file /config/logs/{{ .Name }}.log
        format json
    }
}
`

	t, err := template.New("app").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	if app.Port == 0 {
		app.Port = 8080 // FrankenPHP default port (non-root)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, app); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// GenerateMainConfig generates the main Caddyfile for Docker container
func (g *ConfigGenerator) GenerateMainConfig(email string) (string, error) {
	tmpl := `# FrankenDeploy Caddy Configuration
# Auto-generated - do not edit directly

{
    # Admin API for hot reload (localhost only inside container)
    admin localhost:2019

    # Let's Encrypt email
    email %s
}

# Import app configurations (mounted at /config/apps in container)
import /config/apps/*.caddy
`

	if email == "" {
		email = "admin@localhost"
	}

	return fmt.Sprintf(tmpl, email), nil
}

// AppConfigFromProject creates AppConfig from project config
func AppConfigFromProject(cfg *config.ProjectConfig, domain string) AppConfig {
	return AppConfig{
		Name:   cfg.Name,
		Domain: domain,
		Port:   8080, // FrankenPHP default port (non-root)
		TLS:    true,
	}
}

// ReloadCommands returns SSH commands to reload Caddy config via docker exec
// This provides zero-downtime config updates using Caddy's Admin API inside the container
func ReloadCommands() []string {
	return []string{
		// Reload Caddy config inside container via Admin API (zero downtime)
		`docker exec caddy caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile && echo "Caddy config reloaded"`,
	}
}

// ReloadCommandsFallback returns fallback reload command if graceful reload fails
func ReloadCommandsFallback() []string {
	return []string{
		// Restart container (brief downtime, last resort)
		`docker restart caddy`,
	}
}

// WriteAppConfigCommands returns SSH commands to write app config and reload
func WriteAppConfigCommands(appName, configContent string) []string {
	return []string{
		// Ensure directory exists
		`mkdir -p /opt/frankendeploy/caddy/apps`,
		// Write config file (escaped for shell)
		fmt.Sprintf(`cat > /opt/frankendeploy/caddy/apps/%s.caddy << 'CADDYEOF'
%s
CADDYEOF`, appName, configContent),
		// Reload Caddy inside container
		`docker exec caddy caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile`,
	}
}

// RemoveAppConfigCommands returns SSH commands to remove app config and reload
func RemoveAppConfigCommands(appName string) []string {
	return []string{
		fmt.Sprintf(`rm -f /opt/frankendeploy/caddy/apps/%s.caddy`, appName),
		`docker exec caddy caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile`,
	}
}
