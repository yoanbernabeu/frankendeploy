package cmd

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
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

// hasCommand returns true if any recorded command contains substr.
func hasCommand(cmds []string, substr string) bool {
	for _, c := range cmds {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

func TestPrepareRelease_IssuesExpectedCommands(t *testing.T) {
	mock := &ssh.MockExecutor{}
	cfg := &config.ProjectConfig{
		Name: "myapp",
		Deploy: config.DeployConfig{
			SharedDirs:  []string{"var/log", "var/sessions"},
			SharedFiles: []string{".env.local"},
		},
	}

	if err := prepareRelease(context.Background(), mock, cfg, "/opt/frankendeploy/apps/myapp", "20260420-120000"); err != nil {
		t.Fatalf("prepareRelease() unexpected error: %v", err)
	}

	wantSubstrings := []string{
		"mkdir -p /opt/frankendeploy/apps/myapp/releases/20260420-120000",
		"mkdir -p /opt/frankendeploy/apps/myapp/shared",
		"mkdir -p /opt/frankendeploy/apps/myapp/shared/var/log",
		"mkdir -p /opt/frankendeploy/apps/myapp/shared/var/sessions",
		"touch /opt/frankendeploy/apps/myapp/shared/.env.local",
	}
	for _, want := range wantSubstrings {
		if !hasCommand(mock.Commands, want) {
			t.Errorf("prepareRelease() missing expected command: %q\nrecorded: %v", want, mock.Commands)
		}
	}
}

func TestPrepareRelease_SurfacesNonZeroExit(t *testing.T) {
	mock := &ssh.MockExecutor{
		ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
			return &ssh.ExecResult{ExitCode: 1, Stderr: "permission denied"}, nil
		},
	}
	cfg := &config.ProjectConfig{
		Name:   "myapp",
		Deploy: config.DeployConfig{},
	}

	err := prepareRelease(context.Background(), mock, cfg, "/opt/frankendeploy/apps/myapp", "t1")
	if err == nil {
		t.Fatal("prepareRelease() expected error on non-zero exit code, got nil")
	}
}

func TestStartNewContainer_BuildsDockerRunCommand(t *testing.T) {
	mock := &ssh.MockExecutor{}
	cfg := &config.ProjectConfig{
		Name: "myapp",
		Deploy: config.DeployConfig{
			SharedDirs:  []string{"var/log"},
			SharedFiles: []string{".env.local"},
		},
	}

	err := startNewContainer(
		context.Background(), mock, cfg,
		"myapp:t1",
		"/opt/frankendeploy/apps/myapp",
		"t1",
		"postgres://user:pwd@myapp-db:5432/db",
		"myapp-new",
	)
	if err != nil {
		t.Fatalf("startNewContainer() unexpected error: %v", err)
	}

	// Should force-remove any leftover temp container first
	if !hasCommand(mock.Commands, "docker rm -f myapp-new") {
		t.Errorf("expected force-remove of temp container, got: %v", mock.Commands)
	}

	// Should issue a docker run with the expected flags
	wantFragments := []string{
		"docker run -d --name myapp-new",
		"--network frankendeploy",
		"--restart unless-stopped",
		"--user 1000:1000",
		"-e SERVER_NAME=:8080",
		"-e APP_ENV=prod",
		"-e DATABASE_URL=",
		"-v /opt/frankendeploy/apps/myapp/shared/var/log:/app/var/log",
		"-v /opt/frankendeploy/apps/myapp/shared/.env.local:/app/.env.local:ro",
		"myapp:t1",
	}
	for _, want := range wantFragments {
		if !hasCommand(mock.Commands, want) {
			t.Errorf("startNewContainer() missing fragment %q\nrecorded: %v", want, mock.Commands)
		}
	}
}

func TestSwapContainers_HappyPath(t *testing.T) {
	mock := &ssh.MockExecutor{}

	err := swapContainers(
		context.Background(), mock,
		"myapp",
		"/opt/frankendeploy/apps/myapp",
		"t1",
		"myapp-new",
	)
	if err != nil {
		t.Fatalf("swapContainers() unexpected error: %v", err)
	}

	wantOrdered := []string{
		"docker rename myapp-new myapp",
		"ln -sfn /opt/frankendeploy/apps/myapp/releases/t1 /opt/frankendeploy/apps/myapp/current",
	}
	for _, want := range wantOrdered {
		if !hasCommand(mock.Commands, want) {
			t.Errorf("swapContainers() missing command %q\nrecorded: %v", want, mock.Commands)
		}
	}
}

func TestSwapContainers_SymlinkFailureSurfaces(t *testing.T) {
	// Guards the bug fix: a non-zero exit code from `ln -sfn` must be surfaced
	// as an error instead of being silently ignored.
	mock := &ssh.MockExecutor{
		ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
			if strings.HasPrefix(command, "ln -sfn ") {
				return &ssh.ExecResult{ExitCode: 1, Stderr: "permission denied"}, nil
			}
			return &ssh.ExecResult{ExitCode: 0}, nil
		},
	}

	err := swapContainers(
		context.Background(), mock,
		"myapp",
		"/opt/frankendeploy/apps/myapp",
		"t1",
		"myapp-new",
	)
	if err == nil {
		t.Fatal("swapContainers() expected error when symlink update fails, got nil")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("expected symlink error, got: %v", err)
	}
}

func TestCleanupOldReleases_ConstructsCorrectCommand(t *testing.T) {
	mock := &ssh.MockExecutor{}

	cleanupOldReleases(context.Background(), mock, "/opt/frankendeploy/apps/myapp", 3)

	want := "cd /opt/frankendeploy/apps/myapp/releases && ls -1t | tail -n +4 | xargs -r rm -rf"
	if !hasCommand(mock.Commands, want) {
		t.Errorf("cleanupOldReleases() missing expected command %q\nrecorded: %v", want, mock.Commands)
	}
}

func TestCleanupOldReleases_DefaultKeepReleases(t *testing.T) {
	mock := &ssh.MockExecutor{}

	cleanupOldReleases(context.Background(), mock, "/opt/frankendeploy/apps/myapp", 0)

	// Default is 5, so tail -n +6
	want := "tail -n +6"
	if !hasCommand(mock.Commands, want) {
		t.Errorf("cleanupOldReleases() should default to keep=5 (tail -n +6), got: %v", mock.Commands)
	}
}

func TestTransferSourceCode_PropagatesContext(t *testing.T) {
	// Guards the bug fix: the function must use the propagated ctx instead of context.Background().
	type ctxKey string
	const key ctxKey = "trace"
	seen := ""

	mock := &ssh.MockExecutor{
		ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
			if v, ok := ctx.Value(key).(string); ok {
				seen = v
			}
			// Return an error so transferSourceCode returns before rsync runs externally.
			return nil, fmt.Errorf("mocked failure to short-circuit rsync")
		},
	}

	ctx := context.WithValue(context.Background(), key, "propagated")
	_ = transferSourceCode(
		ctx, mock,
		&config.ServerConfig{Host: "example", User: "deploy", Port: 22},
		"myapp",
		"/opt/frankendeploy/apps/myapp",
	)

	if seen != "propagated" {
		t.Errorf("transferSourceCode() did not propagate ctx to client.Exec; seen=%q", seen)
	}
}

func TestRunHealthCheckOnContainer_RejectsInvalidHealthPath(t *testing.T) {
	mock := &ssh.MockExecutor{}
	cfg := &config.ProjectConfig{
		Name: "myapp",
		Deploy: config.DeployConfig{
			HealthcheckPath: "../../etc/passwd",
		},
	}

	err := runHealthCheckOnContainer(context.Background(), mock, cfg, "myapp-new")
	if err == nil {
		t.Fatal("runHealthCheckOnContainer() expected error for invalid health path, got nil")
	}
	if len(mock.Commands) != 0 {
		t.Errorf("runHealthCheckOnContainer() should not execute any command on invalid path, got: %v", mock.Commands)
	}
}
