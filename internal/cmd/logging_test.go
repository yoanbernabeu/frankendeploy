package cmd

// Tests for issue #53: log rotation and resource limits on the app container.

import (
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
)

func TestBuildAppRunCommand_LogRotation(t *testing.T) {
	cfg := &config.ProjectConfig{Name: "myapp"}
	cmd := buildAppRunCommand(cfg, "myapp:t1", "/opt/frankendeploy/apps/myapp", "", "myapp-new")

	for _, want := range []string{"--log-driver json-file", "--log-opt max-size=10m", "--log-opt max-file=3"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("docker run must rotate logs (%q missing):\n%s", want, cmd)
		}
	}
}

func TestBuildAppRunCommand_ResourceLimits(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "myapp",
		Deploy: config.DeployConfig{
			MemoryLimit: "512m",
			CPULimit:    "1.5",
		},
	}
	cmd := buildAppRunCommand(cfg, "myapp:t1", "/opt/frankendeploy/apps/myapp", "", "myapp-new")

	if !strings.Contains(cmd, "--memory 512m") {
		t.Errorf("docker run should apply deploy.memory_limit:\n%s", cmd)
	}
	if !strings.Contains(cmd, "--cpus 1.5") {
		t.Errorf("docker run should apply deploy.cpu_limit:\n%s", cmd)
	}
}

func TestBuildAppRunCommand_NoLimitsByDefault(t *testing.T) {
	cfg := &config.ProjectConfig{Name: "myapp"}
	cmd := buildAppRunCommand(cfg, "myapp:t1", "/opt/frankendeploy/apps/myapp", "", "myapp-new")

	if strings.Contains(cmd, "--memory") || strings.Contains(cmd, "--cpus") {
		t.Errorf("no resource limit expected by default:\n%s", cmd)
	}
}
