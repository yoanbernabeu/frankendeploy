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

// indexOfCommand returns the index of the first recorded command containing
// substr, or -1 if none matches.
func indexOfCommand(cmds []string, substr string) int {
	for i, c := range cmds {
		if strings.Contains(c, substr) {
			return i
		}
	}
	return -1
}

func TestSwapContainers_RenameBasedOrder(t *testing.T) {
	// Guards the zero-downtime swap (#42): the old container must be renamed
	// away (still running, still serving in-flight requests) and the new one
	// renamed into place BEFORE the old one is stopped. The old stop→rm→rename
	// order left a multi-second window with no container behind the app name.
	mock := &ssh.MockExecutor{}

	err := swapContainers(
		context.Background(), mock,
		"myapp",
		"/opt/frankendeploy/apps/myapp",
		"t1",
		"myapp-new",
		true, // old container exists
	)
	if err != nil {
		t.Fatalf("swapContainers() unexpected error: %v", err)
	}

	renameOld := indexOfCommand(mock.Commands, "docker rename myapp myapp-old")
	renameNew := indexOfCommand(mock.Commands, "docker rename myapp-new myapp")
	stopOld := indexOfCommand(mock.Commands, "docker stop myapp-old")
	symlink := indexOfCommand(mock.Commands, "ln -sfn /opt/frankendeploy/apps/myapp/releases/t1 /opt/frankendeploy/apps/myapp/current")

	if renameOld == -1 || renameNew == -1 || stopOld == -1 || symlink == -1 {
		t.Fatalf("missing expected commands\nrecorded: %v", mock.Commands)
	}
	if !(renameOld < renameNew && renameNew < stopOld) {
		t.Errorf("wrong order: rename old (%d) must precede rename new (%d), which must precede stop old (%d)\nrecorded: %v",
			renameOld, renameNew, stopOld, mock.Commands)
	}
}

func TestSwapContainers_FirstDeployNoOldContainer(t *testing.T) {
	mock := &ssh.MockExecutor{}

	err := swapContainers(
		context.Background(), mock,
		"myapp",
		"/opt/frankendeploy/apps/myapp",
		"t1",
		"myapp-new",
		false, // first deploy: no old container
	)
	if err != nil {
		t.Fatalf("swapContainers() unexpected error: %v", err)
	}

	if idx := indexOfCommand(mock.Commands, "docker rename myapp myapp-old"); idx != -1 {
		t.Errorf("must not rename a non-existent old container, got: %v", mock.Commands)
	}
	if idx := indexOfCommand(mock.Commands, "docker stop myapp-old"); idx != -1 {
		t.Errorf("must not stop a non-existent old container, got: %v", mock.Commands)
	}
	if idx := indexOfCommand(mock.Commands, "docker rename myapp-new myapp"); idx == -1 {
		t.Errorf("expected rename of the new container, got: %v", mock.Commands)
	}
}

func TestSwapContainers_RestoresOldOnRenameFailure(t *testing.T) {
	// Guards the recovery path (#42): if renaming the new container into place
	// fails, the old container must be renamed back so the site keeps being
	// served, instead of staying down with no container behind the app name.
	mock := &ssh.MockExecutor{
		ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
			if strings.Contains(command, "docker rename myapp-new myapp") {
				return &ssh.ExecResult{ExitCode: 1, Stderr: "name already in use"}, nil
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
		true,
	)
	if err == nil {
		t.Fatal("swapContainers() expected error when rename fails, got nil")
	}
	if idx := indexOfCommand(mock.Commands, "docker rename myapp-old myapp"); idx == -1 {
		t.Errorf("expected restore of the old container after failed swap, got: %v", mock.Commands)
	}
}

func TestSwapContainers_AbortsIfOldRenameFails(t *testing.T) {
	// If the old container cannot be renamed away, the swap must abort before
	// touching anything: the old container keeps its name and keeps serving.
	mock := &ssh.MockExecutor{
		ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
			if strings.Contains(command, "docker rename myapp myapp-old") {
				return &ssh.ExecResult{ExitCode: 1, Stderr: "container is restarting"}, nil
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
		true,
	)
	if err == nil {
		t.Fatal("swapContainers() expected error when old rename fails, got nil")
	}
	if idx := indexOfCommand(mock.Commands, "docker rename myapp-new myapp"); idx != -1 {
		t.Errorf("must not rename the new container after a failed old rename, got: %v", mock.Commands)
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
		false,
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

func TestRunHealthCheckOnContainer_UsesConfiguredRetries(t *testing.T) {
	oldDelay := preHealthDelay
	preHealthDelay = 0
	defer func() { preHealthDelay = oldDelay }()

	curlAttempts := 0
	mock := &ssh.MockExecutor{
		ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
			if strings.Contains(command, "docker inspect") {
				return &ssh.ExecResult{Stdout: "running", ExitCode: 0}, nil
			}
			if strings.Contains(command, "curl") {
				curlAttempts++
				return &ssh.ExecResult{Stdout: "500", ExitCode: 1}, nil
			}
			return &ssh.ExecResult{}, nil
		},
	}

	cfg := &config.ProjectConfig{
		Name: "test-app",
		Deploy: config.DeployConfig{
			HealthcheckRetries:  2,
			HealthcheckInterval: 1,
			HealthcheckTimeout:  30,
		},
	}

	err := runHealthCheckOnContainer(context.Background(), mock, cfg, "test-app-new")
	if err == nil {
		t.Fatal("expected health check failure")
	}
	if curlAttempts != 2 {
		t.Errorf("expected 2 attempts from config, got %d", curlAttempts)
	}
}
