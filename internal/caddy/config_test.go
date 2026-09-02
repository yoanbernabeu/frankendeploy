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

// TestGenerateAppConfig_NoActiveHealthCheck guards against reintroducing
// Caddy's active health check. With a single upstream it cannot route around
// anything, and it has caused two production outages: a probe on "/" getting
// 404 from an API-only app, then a probe timing out under load, both turning
// every request into a 503 "no upstreams available".
func TestGenerateAppConfig_NoActiveHealthCheck(t *testing.T) {
	gen := NewConfigGenerator()
	out, err := gen.GenerateAppConfig(AppConfig{
		Name:   "myapp",
		Domain: "example.com",
		Port:   8080,
	})
	if err != nil {
		t.Fatalf("GenerateAppConfig: %v", err)
	}
	for _, forbidden := range []string{"health_uri", "health_interval", "health_timeout"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("generated config must not contain %q:\n%s", forbidden, out)
		}
	}
	if !strings.Contains(out, "reverse_proxy myapp:8080\n") {
		t.Errorf("expected a bare reverse_proxy directive:\n%s", out)
	}
}

func TestAppConfigFromProject(t *testing.T) {
	cfg := &config.ProjectConfig{Name: "myapp"}

	app := AppConfigFromProject(cfg, "example.com")
	if app.Name != "myapp" || app.Domain != "example.com" || app.Port == 0 {
		t.Errorf("unexpected AppConfig: %+v", app)
	}
}

func TestReloadCommands(t *testing.T) {
	cmds := ReloadCommands()
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if !strings.Contains(cmds[0], "docker exec caddy caddy reload") {
		t.Errorf("expected caddy reload command, got: %s", cmds[0])
	}
	if !strings.Contains(cmds[0], "--config /etc/caddy/Caddyfile") {
		t.Errorf("reload must target the main Caddyfile, got: %s", cmds[0])
	}
}

func TestWriteAppConfigCommands(t *testing.T) {
	content := "example.com {\n    reverse_proxy myapp:80\n}\n"
	cmds, err := WriteAppConfigCommands("myapp", content)
	if err != nil {
		t.Fatalf("WriteAppConfigCommands: %v", err)
	}
	if len(cmds) != 3 {
		t.Fatalf("expected 3 commands (mkdir, write, reload), got %d", len(cmds))
	}
	if !strings.HasPrefix(cmds[0], "mkdir -p ") {
		t.Errorf("first command should create the apps dir, got: %s", cmds[0])
	}
	if !strings.Contains(cmds[1], "myapp.caddy") || !strings.Contains(cmds[1], content) {
		t.Errorf("second command should write the app config content, got: %s", cmds[1])
	}
	if !strings.Contains(cmds[2], "caddy reload") {
		t.Errorf("third command should reload Caddy, got: %s", cmds[2])
	}

	// Heredoc delimiter must be random: two calls must differ
	cmds2, err := WriteAppConfigCommands("myapp", content)
	if err != nil {
		t.Fatalf("WriteAppConfigCommands: %v", err)
	}
	if cmds[1] == cmds2[1] {
		t.Error("heredoc delimiter should be randomized between calls")
	}
}

func TestRemoveAppConfigCommands(t *testing.T) {
	cmds := RemoveAppConfigCommands("myapp")
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands (rm, reload), got %d", len(cmds))
	}
	if !strings.Contains(cmds[0], "rm -f") || !strings.Contains(cmds[0], "myapp.caddy") {
		t.Errorf("first command should remove the app config, got: %s", cmds[0])
	}
	if !strings.Contains(cmds[1], "caddy reload") {
		t.Errorf("second command should reload Caddy, got: %s", cmds[1])
	}
}

func TestGenerateAppConfig_IncludesHSTS(t *testing.T) {
	gen := NewConfigGenerator()
	out, err := gen.GenerateAppConfig(AppConfig{
		Name:   "myapp",
		Domain: "example.com",
		Port:   8080,
	})
	if err != nil {
		t.Fatalf("GenerateAppConfig: %v", err)
	}
	if !strings.Contains(out, `Strict-Transport-Security "max-age=31536000"`) {
		t.Errorf("expected HSTS header in generated config:\n%s", out)
	}
}

func TestGenerateAppConfig_DoesNotEmitDeprecatedXXSSProtection(t *testing.T) {
	gen := NewConfigGenerator()
	out, err := gen.GenerateAppConfig(AppConfig{
		Name:   "myapp",
		Domain: "example.com",
		Port:   8080,
	})
	if err != nil {
		t.Fatalf("GenerateAppConfig: %v", err)
	}
	if strings.Contains(out, "X-XSS-Protection") {
		t.Errorf("generated config must not contain deprecated X-XSS-Protection:\n%s", out)
	}
}

func TestGenerateAppConfig_LogRotation(t *testing.T) {
	gen := NewConfigGenerator()
	out, err := gen.GenerateAppConfig(AppConfig{
		Name:   "myapp",
		Domain: "example.com",
		Port:   8080,
	})
	if err != nil {
		t.Fatalf("GenerateAppConfig: %v", err)
	}
	for _, want := range []string{"roll_size 10mb", "roll_keep 5"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in log block (unbounded Caddy access logs fill the disk):\n%s", want, out)
		}
	}
}
