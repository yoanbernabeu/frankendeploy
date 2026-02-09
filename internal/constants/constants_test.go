package constants

import "testing"

func TestAppBasePath(t *testing.T) {
	tests := []struct {
		name     string
		appName  string
		expected string
	}{
		{"simple name", "myapp", "/opt/frankendeploy/apps/myapp"},
		{"hyphenated name", "my-app", "/opt/frankendeploy/apps/my-app"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AppBasePath(tt.appName)
			if got != tt.expected {
				t.Errorf("AppBasePath(%q) = %q, want %q", tt.appName, got, tt.expected)
			}
		})
	}
}

func TestAppReleasePath(t *testing.T) {
	got := AppReleasePath("myapp", "20240115-120000")
	expected := "/opt/frankendeploy/apps/myapp/releases/20240115-120000"
	if got != expected {
		t.Errorf("AppReleasePath() = %q, want %q", got, expected)
	}
}

func TestAppCurrentPath(t *testing.T) {
	got := AppCurrentPath("myapp")
	expected := "/opt/frankendeploy/apps/myapp/current"
	if got != expected {
		t.Errorf("AppCurrentPath() = %q, want %q", got, expected)
	}
}

func TestAppSharedPath(t *testing.T) {
	got := AppSharedPath("myapp")
	expected := "/opt/frankendeploy/apps/myapp/shared"
	if got != expected {
		t.Errorf("AppSharedPath() = %q, want %q", got, expected)
	}
}

func TestCaddyAppConfig(t *testing.T) {
	got := CaddyAppConfig("myapp")
	expected := "/opt/frankendeploy/caddy/apps/myapp.caddy"
	if got != expected {
		t.Errorf("CaddyAppConfig() = %q, want %q", got, expected)
	}
}

func TestAppEnvFilePath(t *testing.T) {
	got := AppEnvFilePath("myapp")
	expected := "/opt/frankendeploy/apps/myapp/shared/.env.local"
	if got != expected {
		t.Errorf("AppEnvFilePath() = %q, want %q", got, expected)
	}
}

func TestConstants(t *testing.T) {
	if BasePath != "/opt/frankendeploy" {
		t.Errorf("BasePath = %q, want /opt/frankendeploy", BasePath)
	}
	if AppsDir != "/opt/frankendeploy/apps" {
		t.Errorf("AppsDir = %q, want /opt/frankendeploy/apps", AppsDir)
	}
	if ContainerUser != "1000:1000" {
		t.Errorf("ContainerUser = %q, want 1000:1000", ContainerUser)
	}
	if AppPort != "8080" {
		t.Errorf("AppPort = %q, want 8080", AppPort)
	}
	if NetworkName != "frankendeploy" {
		t.Errorf("NetworkName = %q, want frankendeploy", NetworkName)
	}
	if DefaultKeepReleases != 5 {
		t.Errorf("DefaultKeepReleases = %d, want 5", DefaultKeepReleases)
	}
}
