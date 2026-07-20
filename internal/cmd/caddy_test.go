package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

func TestCaddyAppConfigExists(t *testing.T) {
	tests := []struct {
		name   string
		stdout string
		want   bool
	}{
		{"config present", "yes\n", true},
		{"config absent", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &ssh.MockExecutor{
				ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
					if !strings.Contains(command, "my-app.caddy") {
						t.Errorf("expected check on my-app.caddy, got: %s", command)
					}
					return &ssh.ExecResult{Stdout: tt.stdout, ExitCode: 0}, nil
				},
			}
			if got := caddyAppConfigExists(context.Background(), mock, "my-app"); got != tt.want {
				t.Errorf("caddyAppConfigExists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUpdateCaddyConfig_FailsWhenCaddyNotRunning(t *testing.T) {
	mock := &ssh.MockExecutor{
		ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
			if strings.Contains(command, "docker inspect caddy") {
				return &ssh.ExecResult{Stdout: "exited\n", ExitCode: 0}, nil
			}
			t.Errorf("no command should run after the caddy status check, got: %s", command)
			return &ssh.ExecResult{}, nil
		},
	}
	cfg := &config.ProjectConfig{
		Name:   "my-app",
		Deploy: config.DeployConfig{Domain: "example.com"},
	}
	err := updateCaddyConfig(context.Background(), mock, cfg)
	if err == nil {
		t.Fatal("expected error when caddy container is not running")
	}
	if !strings.Contains(err.Error(), "server setup") {
		t.Errorf("error should point the user to 'server setup', got: %v", err)
	}
}

func TestUpdateCaddyConfig_SucceedsWhenCaddyRunning(t *testing.T) {
	var commands []string
	mock := &ssh.MockExecutor{
		ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
			commands = append(commands, command)
			if strings.Contains(command, "docker inspect caddy") {
				return &ssh.ExecResult{Stdout: "running\n", ExitCode: 0}, nil
			}
			return &ssh.ExecResult{ExitCode: 0}, nil
		},
	}
	cfg := &config.ProjectConfig{
		Name:   "my-app",
		Deploy: config.DeployConfig{Domain: "example.com", HealthcheckPath: "/api"},
	}
	if err := updateCaddyConfig(context.Background(), mock, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	joined := strings.Join(commands, "\n")
	if !strings.Contains(joined, "my-app.caddy") || !strings.Contains(joined, "caddy reload") {
		t.Errorf("expected write and reload commands, got:\n%s", joined)
	}
}
