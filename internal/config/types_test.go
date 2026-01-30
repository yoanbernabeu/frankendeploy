package config

import (
	"testing"
)

func TestDefaultProjectConfig(t *testing.T) {
	cfg := DefaultProjectConfig()

	if cfg.PHP.Version != "8.3" {
		t.Errorf("expected PHP version 8.3, got %s", cfg.PHP.Version)
	}

	if cfg.Deploy.KeepReleases != 5 {
		t.Errorf("expected keep_releases 5, got %d", cfg.Deploy.KeepReleases)
	}

	if len(cfg.PHP.Extensions) == 0 {
		t.Error("expected default extensions")
	}
}

func TestDefaultGlobalConfig(t *testing.T) {
	cfg := DefaultGlobalConfig()

	if cfg.Servers == nil {
		t.Error("expected servers map to be initialized")
	}

	if cfg.DefaultPort != 22 {
		t.Errorf("expected default port 22, got %d", cfg.DefaultPort)
	}

	if cfg.DefaultUser != "deploy" {
		t.Errorf("expected default user 'deploy', got %s", cfg.DefaultUser)
	}
}
