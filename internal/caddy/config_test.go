package caddy

import (
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
)

func TestGenerateAppConfig_DoesNotEmitTLSInternal(t *testing.T) {
	// The `tls internal` branch was unreachable since AppConfigFromProject
	// always left TLS as the default. The field and branch were removed —
	// this test guards against accidental reintroduction.
	gen := NewConfigGenerator()
	out, err := gen.GenerateAppConfig(AppConfig{
		Name:   "myapp",
		Domain: "example.com",
		Port:   8080,
	})
	if err != nil {
		t.Fatalf("GenerateAppConfig: %v", err)
	}
	if strings.Contains(out, "tls internal") {
		t.Errorf("generated config should not contain 'tls internal':\n%s", out)
	}
	wantFragments := []string{
		"example.com {",
		"reverse_proxy myapp:8080",
		"encode zstd gzip",
	}
	for _, want := range wantFragments {
		if !strings.Contains(out, want) {
			t.Errorf("generated config missing %q\n%s", want, out)
		}
	}
}

// TestGenerateAppConfig_UsesConfiguredHealthPath guards against the live 503
// found in production: Caddy's active health check probed a hardcoded "/",
// got 404 from an API-only app, marked the upstream unhealthy, and every
// request failed with "no upstreams available".
func TestGenerateAppConfig_UsesConfiguredHealthPath(t *testing.T) {
	gen := NewConfigGenerator()
	out, err := gen.GenerateAppConfig(AppConfig{
		Name:       "myapp",
		Domain:     "example.com",
		Port:       8080,
		HealthPath: "/api",
	})
	if err != nil {
		t.Fatalf("GenerateAppConfig: %v", err)
	}
	if !strings.Contains(out, "health_uri /api") {
		t.Errorf("expected 'health_uri /api' in generated config:\n%s", out)
	}
}

func TestGenerateAppConfig_DefaultsHealthPathToRoot(t *testing.T) {
	gen := NewConfigGenerator()
	out, err := gen.GenerateAppConfig(AppConfig{
		Name:   "myapp",
		Domain: "example.com",
		Port:   8080,
	})
	if err != nil {
		t.Fatalf("GenerateAppConfig: %v", err)
	}
	if !strings.Contains(out, "health_uri /") {
		t.Errorf("expected 'health_uri /' as default:\n%s", out)
	}
}

func TestGenerateAppConfig_RejectsInvalidHealthPath(t *testing.T) {
	gen := NewConfigGenerator()
	for _, path := range []string{"no-leading-slash", "/api\nmalicious {", "/api/../../etc"} {
		_, err := gen.GenerateAppConfig(AppConfig{
			Name:       "myapp",
			Domain:     "example.com",
			Port:       8080,
			HealthPath: path,
		})
		if err == nil {
			t.Errorf("expected error for invalid health path %q", path)
		}
	}
}

func TestAppConfigFromProject_CopiesHealthcheckPath(t *testing.T) {
	cfg := &config.ProjectConfig{Name: "myapp"}
	cfg.Deploy.HealthcheckPath = "/api"

	app := AppConfigFromProject(cfg, "example.com")
	if app.HealthPath != "/api" {
		t.Errorf("expected HealthPath /api, got %q", app.HealthPath)
	}
}
