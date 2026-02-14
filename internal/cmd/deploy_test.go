package cmd

import (
	"strings"
	"testing"
)

func TestBuildVolumeMounts(t *testing.T) {
	tests := []struct {
		name        string
		sharedPath  string
		sharedDirs  []string
		sharedFiles []string
		wantParts   []string
	}{
		{
			name:        "default shared dirs and files",
			sharedPath:  "/opt/frankendeploy/apps/myapp/shared",
			sharedDirs:  []string{"var/log", "var/sessions"},
			sharedFiles: []string{".env.local"},
			wantParts: []string{
				"-v /opt/frankendeploy/apps/myapp/shared/var/log:/app/var/log",
				"-v /opt/frankendeploy/apps/myapp/shared/var/sessions:/app/var/sessions",
				"-v /opt/frankendeploy/apps/myapp/shared/.env.local:/app/.env.local:ro",
			},
		},
		{
			name:        "custom dirs only",
			sharedPath:  "/data/shared",
			sharedDirs:  []string{"uploads"},
			sharedFiles: []string{},
			wantParts: []string{
				"-v /data/shared/uploads:/app/uploads",
			},
		},
		{
			name:        "env file gets ro mode",
			sharedPath:  "/shared",
			sharedDirs:  []string{},
			sharedFiles: []string{".env.local", "config/custom.yaml"},
			wantParts: []string{
				"-v /shared/.env.local:/app/.env.local:ro",
				"-v /shared/config/custom.yaml:/app/config/custom.yaml",
			},
		},
		{
			name:        "empty dirs and files",
			sharedPath:  "/shared",
			sharedDirs:  []string{},
			sharedFiles: []string{},
			wantParts:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildVolumeMounts(tt.sharedPath, tt.sharedDirs, tt.sharedFiles)

			for _, part := range tt.wantParts {
				if !strings.Contains(result, part) {
					t.Errorf("buildVolumeMounts() missing expected part: %s\ngot: %s", part, result)
				}
			}

			if len(tt.wantParts) == 0 && result != "" {
				t.Errorf("buildVolumeMounts() = %q, expected empty", result)
			}
		})
	}
}
