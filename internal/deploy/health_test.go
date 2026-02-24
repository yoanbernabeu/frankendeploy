package deploy

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

func TestHealthChecker_Check_Healthy(t *testing.T) {
	mock := &ssh.MockExecutor{
		ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
			if strings.Contains(command, "docker inspect") {
				return &ssh.ExecResult{Stdout: "running", ExitCode: 0}, nil
			}
			if strings.Contains(command, "curl") {
				return &ssh.ExecResult{Stdout: "200", ExitCode: 0}, nil
			}
			return &ssh.ExecResult{}, nil
		},
	}

	hc := NewHealthChecker(mock, "test-app", "/healthz", "8080")
	hc.SetTimeout(10 * time.Second)
	hc.SetRetries(3)
	hc.SetInterval(100 * time.Millisecond)

	result, err := hc.Check(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Healthy {
		t.Errorf("expected healthy, got message: %s", result.Message)
	}
	if result.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", result.Attempts)
	}
	if result.StatusCode != 200 {
		t.Errorf("expected status code 200, got %d", result.StatusCode)
	}
}

func TestHealthChecker_Check_ParsesActualStatusCode(t *testing.T) {
	mock := &ssh.MockExecutor{
		ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
			if strings.Contains(command, "docker inspect") {
				return &ssh.ExecResult{Stdout: "running", ExitCode: 0}, nil
			}
			if strings.Contains(command, "curl") {
				// Simulate a 204 No Content response
				return &ssh.ExecResult{Stdout: "204", ExitCode: 0}, nil
			}
			return &ssh.ExecResult{}, nil
		},
	}

	hc := NewHealthChecker(mock, "test-app", "/healthz", "8080")
	hc.SetTimeout(10 * time.Second)
	hc.SetRetries(3)
	hc.SetInterval(100 * time.Millisecond)

	result, err := hc.Check(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Healthy {
		t.Errorf("expected healthy, got message: %s", result.Message)
	}
	if result.StatusCode != 204 {
		t.Errorf("expected status code 204, got %d", result.StatusCode)
	}
}

func TestHealthChecker_Check_ContainerNotRunning(t *testing.T) {
	mock := &ssh.MockExecutor{
		ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
			if strings.Contains(command, "docker inspect") {
				return &ssh.ExecResult{Stdout: "exited", ExitCode: 0}, nil
			}
			return &ssh.ExecResult{}, nil
		},
	}

	hc := NewHealthChecker(mock, "test-app", "/", "8080")
	hc.SetTimeout(2 * time.Second)
	hc.SetRetries(2)
	hc.SetInterval(100 * time.Millisecond)

	result, err := hc.Check(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Healthy {
		t.Error("expected unhealthy")
	}
	if !strings.Contains(result.Message, "container not running") {
		t.Errorf("expected 'container not running' message, got: %s", result.Message)
	}
}

func TestHealthChecker_Check_Timeout(t *testing.T) {
	mock := &ssh.MockExecutor{
		ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
			if strings.Contains(command, "docker inspect") {
				return &ssh.ExecResult{Stdout: "running", ExitCode: 0}, nil
			}
			// HTTP check always fails
			return &ssh.ExecResult{Stdout: "503", ExitCode: 1}, fmt.Errorf("command failed (exit 1)")
		},
	}

	hc := NewHealthChecker(mock, "test-app", "/", "8080")
	hc.SetTimeout(500 * time.Millisecond)
	hc.SetRetries(10)
	hc.SetInterval(100 * time.Millisecond)

	result, err := hc.Check(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Healthy {
		t.Error("expected unhealthy due to timeout")
	}
}

func TestHealthChecker_Check_PortInCurlCommand(t *testing.T) {
	var capturedCmd string
	mock := &ssh.MockExecutor{
		ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
			if strings.Contains(command, "docker inspect") {
				return &ssh.ExecResult{Stdout: "running", ExitCode: 0}, nil
			}
			if strings.Contains(command, "curl") {
				capturedCmd = command
				return &ssh.ExecResult{Stdout: "200", ExitCode: 0}, nil
			}
			return &ssh.ExecResult{}, nil
		},
	}

	hc := NewHealthChecker(mock, "test-app", "/health", "8080")
	hc.SetRetries(1)

	_, err := hc.Check(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(capturedCmd, "localhost:8080/health") {
		t.Errorf("expected curl command to contain 'localhost:8080/health', got: %s", capturedCmd)
	}
}

func TestHealthChecker_CheckContainer(t *testing.T) {
	tests := []struct {
		name     string
		stdout   string
		expected bool
	}{
		{"running container", "Up 5 minutes", true},
		{"stopped container", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &ssh.MockExecutor{
				ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
					return &ssh.ExecResult{Stdout: tt.stdout, ExitCode: 0}, nil
				},
			}
			hc := NewHealthChecker(mock, "test-app", "/", "8080")
			running, err := hc.CheckContainer(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if running != tt.expected {
				t.Errorf("expected running=%v, got %v", tt.expected, running)
			}
		})
	}
}
