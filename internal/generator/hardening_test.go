package generator

// Tests for issue #48: production Dockerfile hardening.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
)

func minimalConfig() *config.ProjectConfig {
	return &config.ProjectConfig{
		Name: "test-app",
		PHP:  config.PHPConfig{Version: "8.3", Extensions: []string{"intl"}},
	}
}

func generateDockerfile(t *testing.T, cfg *config.ProjectConfig) string {
	t.Helper()
	dockerfile, err := NewDockerfileGenerator(cfg).Generate()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	return dockerfile
}

func TestDockerfile_NoSilentBuildFailures(t *testing.T) {
	dockerfile := generateDockerfile(t, minimalConfig())

	if strings.Contains(dockerfile, "post-install-cmd || true") {
		t.Error("composer post-install-cmd must not be silenced with || true: a failing build script must fail the image build")
	}
	if !strings.Contains(dockerfile, "post-install-cmd") {
		t.Error("Dockerfile should still run post-install-cmd")
	}
	if !strings.Contains(dockerfile, "cache:warmup") {
		t.Error("Dockerfile should explicitly warm the Symfony cache at build time")
	}
}

func TestDockerfile_NoVolumeVar(t *testing.T) {
	dockerfile := generateDockerfile(t, minimalConfig())

	if strings.Contains(dockerfile, "VOLUME") {
		t.Error("Dockerfile must not declare VOLUME /app/var: it can discard build-time cache warmup with the classic builder")
	}
}

func TestDockerfile_ProdOpcacheIni(t *testing.T) {
	dockerfile := generateDockerfile(t, minimalConfig())

	for _, want := range []string{
		"app.prod.ini",
		"opcache.validate_timestamps = 0",
		"opcache.memory_consumption",
		"opcache.max_accelerated_files",
		"realpath_cache_size",
	} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("Dockerfile should contain production PHP setting %q", want)
		}
	}

	// The prod ini must live in the prod stage, not the base stage (shared
	// with dev): validate_timestamps=0 in dev would require container
	// restarts on every code change.
	prodStage := strings.Index(dockerfile, "AS frankenphp_prod")
	iniPos := strings.Index(dockerfile, "opcache.validate_timestamps")
	if prodStage == -1 || iniPos < prodStage {
		t.Error("production OPcache settings must be in the frankenphp_prod stage only")
	}
}

func TestDockerfile_PreloadWhenConfigExists(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "preload.php"), []byte("<?php"), 0644); err != nil {
		t.Fatalf("failed to write preload.php: %v", err)
	}
	t.Chdir(dir)

	dockerfile := generateDockerfile(t, minimalConfig())
	if !strings.Contains(dockerfile, "opcache.preload = /app/config/preload.php") {
		t.Error("Dockerfile should enable opcache.preload when config/preload.php exists")
	}
}

func TestDockerfile_NoPreloadWithoutConfig(t *testing.T) {
	t.Chdir(t.TempDir())

	dockerfile := generateDockerfile(t, minimalConfig())
	if strings.Contains(dockerfile, "opcache.preload") {
		t.Error("Dockerfile must not enable opcache.preload when config/preload.php does not exist")
	}
}

func TestDockerfile_NoFullAppChown(t *testing.T) {
	dockerfile := generateDockerfile(t, minimalConfig())

	if strings.Contains(dockerfile, "chown -R app:app /app;") {
		t.Error("Dockerfile must not chown -R the whole /app in a RUN layer: it duplicates every file and doubles the image size")
	}

	wantCopy := fmt.Sprintf("COPY --link --chown=%s:%s . ./", DefaultUID, DefaultGID)
	if !strings.Contains(dockerfile, wantCopy) {
		t.Errorf("Dockerfile should copy sources with %q instead of a separate chown layer", wantCopy)
	}

	// Files created by root during the build RUN (cache warmup) still need
	// app ownership, but only var/ — a small tree.
	if !strings.Contains(dockerfile, "chown -R app:app var") {
		t.Error("Dockerfile should chown var/ (created during build) to the app user")
	}
}

func TestDockerfile_NodeVersionDefault(t *testing.T) {
	cfg := minimalConfig()
	cfg.Assets = config.AssetsConfig{BuildTool: "npm", BuildCommand: "npm run build"}

	dockerfile := generateDockerfile(t, cfg)
	if !strings.Contains(dockerfile, "FROM node:22-slim AS node_build") {
		t.Error("default Node version should be 22 (Node 20 EOL April 2026)")
	}
}

func TestDockerfile_NodeVersionConfigurable(t *testing.T) {
	cfg := minimalConfig()
	cfg.Assets = config.AssetsConfig{BuildTool: "npm", BuildCommand: "npm run build", NodeVersion: "24"}

	dockerfile := generateDockerfile(t, cfg)
	if !strings.Contains(dockerfile, "FROM node:24-slim AS node_build") {
		t.Error("assets.node_version should override the Node image version")
	}
}

func TestDockerfile_NodeVersionRejectsUnsafe(t *testing.T) {
	for _, bad := range []string{"22-slim; RUN evil", "22 AS x", "$(cat)", "latest\nRUN evil"} {
		t.Run(bad, func(t *testing.T) {
			cfg := minimalConfig()
			cfg.Assets = config.AssetsConfig{BuildTool: "npm", BuildCommand: "npm run build", NodeVersion: bad}
			if _, err := NewDockerfileGenerator(cfg).Generate(); err == nil {
				t.Errorf("expected error for unsafe node_version %q, got nil", bad)
			}
		})
	}
}

func TestDockerfile_NoPersistentComposerSuperuser(t *testing.T) {
	dockerfile := generateDockerfile(t, minimalConfig())

	if strings.Contains(dockerfile, "ENV COMPOSER_ALLOW_SUPERUSER") {
		t.Error("COMPOSER_ALLOW_SUPERUSER must not persist as ENV in the final image")
	}
	if !strings.Contains(dockerfile, "COMPOSER_ALLOW_SUPERUSER=1 composer") {
		t.Error("composer invocations during build should set COMPOSER_ALLOW_SUPERUSER inline")
	}
}

func TestDockerfile_AppLevelHealthcheck(t *testing.T) {
	dockerfile := generateDockerfile(t, minimalConfig())

	if strings.Contains(dockerfile, "2019/metrics") {
		t.Error("HEALTHCHECK must not hit the Caddy admin endpoint: it only proves Caddy is up, not the app")
	}
	want := fmt.Sprintf("http://localhost:%s/", AppPort)
	if !strings.Contains(dockerfile, want) {
		t.Errorf("HEALTHCHECK should hit the app on %q", want)
	}
}

func TestDockerfile_HealthcheckCustomPath(t *testing.T) {
	cfg := minimalConfig()
	cfg.Deploy.HealthcheckPath = "/health"

	dockerfile := generateDockerfile(t, cfg)
	want := fmt.Sprintf("http://localhost:%s/health", AppPort)
	if !strings.Contains(dockerfile, want) {
		t.Errorf("HEALTHCHECK should use the configured healthcheck_path, want %q", want)
	}
}

func TestDockerfile_HealthcheckPathRejectsUnsafe(t *testing.T) {
	for _, bad := range []string{"no-leading-slash", "/path; rm -rf /", "/path$(evil)", "/path space", "/path\nRUN evil"} {
		t.Run(bad, func(t *testing.T) {
			cfg := minimalConfig()
			cfg.Deploy.HealthcheckPath = bad
			if _, err := NewDockerfileGenerator(cfg).Generate(); err == nil {
				t.Errorf("expected error for unsafe healthcheck_path %q, got nil", bad)
			}
		})
	}
}

func TestComposeProd_AppLevelHealthcheck(t *testing.T) {
	cfg := minimalConfig()
	cfg.Deploy.HealthcheckPath = "/health"

	compose, err := NewComposeGenerator(cfg).GenerateProd()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	if strings.Contains(compose, "2019/metrics") {
		t.Error("prod compose healthcheck must not hit the Caddy admin endpoint")
	}
	want := fmt.Sprintf("http://localhost:%s/health", AppPort)
	if !strings.Contains(compose, want) {
		t.Errorf("prod compose healthcheck should hit the app on %q", want)
	}
}

func TestComposeDev_AppLevelHealthcheck(t *testing.T) {
	compose, err := NewComposeGenerator(minimalConfig()).GenerateDev()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	if strings.Contains(compose, "2019/metrics") {
		t.Error("dev compose healthcheck must not hit the Caddy admin endpoint")
	}
	want := fmt.Sprintf("http://localhost:%s/", AppPort)
	if !strings.Contains(compose, want) {
		t.Errorf("dev compose healthcheck should hit the app on %q", want)
	}
}
