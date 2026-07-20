package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

func TestBuildPlatformForServer(t *testing.T) {
	tests := []struct {
		name     string
		stdout   string
		exitCode int
		execErr  error
		want     string
	}{
		{name: "x86_64 server", stdout: "x86_64\n", want: "linux/amd64"},
		{name: "aarch64 server", stdout: "aarch64\n", want: "linux/arm64"},
		{name: "arm64 server", stdout: "arm64\n", want: "linux/arm64"},
		{name: "amd64 server", stdout: "amd64\n", want: "linux/amd64"},
		{name: "exec error falls back to amd64", execErr: errors.New("connection lost"), want: "linux/amd64"},
		{name: "non-zero exit falls back to amd64", stdout: "", exitCode: 127, want: "linux/amd64"},
		{name: "unknown arch falls back to amd64", stdout: "riscv64\n", want: "linux/amd64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &ssh.MockExecutor{
				ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
					if tt.execErr != nil {
						return nil, tt.execErr
					}
					return &ssh.ExecResult{Stdout: tt.stdout, ExitCode: tt.exitCode}, nil
				},
			}
			got := buildPlatformForServer(context.Background(), mock)
			if got != tt.want {
				t.Errorf("buildPlatformForServer() = %q, want %q", got, tt.want)
			}
			if len(mock.Commands) != 1 || mock.Commands[0] != "uname -m" {
				t.Errorf("expected a single 'uname -m' command, got %v", mock.Commands)
			}
		})
	}
}
