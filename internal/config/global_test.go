package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGlobalConfig_RejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "servers:\n  prod:\n    host: example.com\n    usre: typo\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadGlobalConfigFromPath(path); err == nil {
		t.Error("a typoed field in the global config must be rejected (KnownFields), not silently ignored")
	}
}

func TestLoadGlobalConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "servers:\n  prod:\n    host: example.com\n    user: deploy\n    port: 22\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadGlobalConfigFromPath(path)
	if err != nil {
		t.Fatalf("LoadGlobalConfigFromPath: %v", err)
	}
	if cfg.Servers["prod"].Host != "example.com" {
		t.Errorf("unexpected host: %+v", cfg.Servers["prod"])
	}
}
