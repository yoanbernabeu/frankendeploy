package caddy

import (
	"bytes"
	"fmt"
	"strconv"
	"text/template"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/constants"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
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
}

// GenerateAppConfig generates Caddy config for an application
func (g *ConfigGenerator) GenerateAppConfig(app AppConfig) (string, error) {
	tmpl := `# {{ .Name }}
{{ .Domain }} {
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
		app.Port, _ = strconv.Atoi(constants.AppPort)
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
		email = constants.DefaultCertEmail
	}

	return fmt.Sprintf(tmpl, email), nil
}

// AppConfigFromProject creates AppConfig from project config
func AppConfigFromProject(cfg *config.ProjectConfig, domain string) AppConfig {
	port, _ := strconv.Atoi(constants.AppPort)
	return AppConfig{
		Name:   cfg.Name,
		Domain: domain,
		Port:   port,
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

// WriteAppConfigCommands returns SSH commands to write app config and reload.
// Returns an error if the heredoc delimiter cannot be generated.
func WriteAppConfigCommands(appName, configContent string) ([]string, error) {
	delim, err := security.GenerateHeredocDelimiter("CADDYEOF")
	if err != nil {
		return nil, fmt.Errorf("failed to generate delimiter: %w", err)
	}
	return []string{
		// Ensure directory exists
		fmt.Sprintf("mkdir -p %s", constants.CaddyAppsDir),
		// Write config file (escaped for shell with random heredoc delimiter)
		fmt.Sprintf("cat > %s << '%s'\n%s\n%s", constants.CaddyAppConfig(appName), delim, configContent, delim),
		// Reload Caddy inside container
		`docker exec caddy caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile`,
	}, nil
}

// RemoveAppConfigCommands returns SSH commands to remove app config and reload
func RemoveAppConfigCommands(appName string) []string {
	return []string{
		fmt.Sprintf("rm -f %s", constants.CaddyAppConfig(appName)),
		`docker exec caddy caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile`,
	}
}
