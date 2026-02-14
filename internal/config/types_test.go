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

func TestEffectiveSharedDirs(t *testing.T) {
	tests := []struct {
		name     string
		config   DeployConfig
		expected []string
	}{
		{
			name:     "returns defaults when empty",
			config:   DeployConfig{},
			expected: DefaultSharedDirs,
		},
		{
			name:     "returns configured dirs",
			config:   DeployConfig{SharedDirs: []string{"custom/dir"}},
			expected: []string{"custom/dir"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.EffectiveSharedDirs()
			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d dirs, got %d", len(tt.expected), len(got))
			}
			for i, v := range got {
				if v != tt.expected[i] {
					t.Errorf("dir[%d]: expected %q, got %q", i, tt.expected[i], v)
				}
			}
		})
	}
}

func TestEffectiveSharedFiles(t *testing.T) {
	tests := []struct {
		name     string
		config   DeployConfig
		expected []string
	}{
		{
			name:     "returns defaults when empty",
			config:   DeployConfig{},
			expected: DefaultSharedFiles,
		},
		{
			name:     "returns configured files",
			config:   DeployConfig{SharedFiles: []string{".env.prod", "config/secrets.yaml"}},
			expected: []string{".env.prod", "config/secrets.yaml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.EffectiveSharedFiles()
			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d files, got %d", len(tt.expected), len(got))
			}
			for i, v := range got {
				if v != tt.expected[i] {
					t.Errorf("file[%d]: expected %q, got %q", i, tt.expected[i], v)
				}
			}
		})
	}
}
