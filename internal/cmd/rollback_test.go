package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

func TestFindPreviousRelease(t *testing.T) {
	tests := []struct {
		name     string
		releases []string
		current  string
		want     string
		wantErr  bool
	}{
		{
			// Guards the mtime-based bug: `ls -1t | head -2 | tail -1` could
			// return a release NEWER than the current one after a rollback.
			name:     "previous of newest",
			releases: []string{"20260101-1000", "20260102-1000", "20260103-1000"},
			current:  "20260103-1000",
			want:     "20260102-1000",
		},
		{
			name:     "second rollback must not bounce forward",
			releases: []string{"20260101-1000", "20260102-1000", "20260103-1000"},
			current:  "20260101-1000",
			wantErr:  true,
		},
		{
			name:     "rollback from middle release",
			releases: []string{"20260101-1000", "20260102-1000", "20260103-1000"},
			current:  "20260102-1000",
			want:     "20260101-1000",
		},
		{
			name:     "unknown current falls back to newest",
			releases: []string{"20260101-1000", "20260102-1000"},
			current:  "",
			want:     "20260102-1000",
		},
		{
			name:     "single release",
			releases: []string{"20260101-1000"},
			current:  "20260101-1000",
			wantErr:  true,
		},
		{
			name:     "no releases",
			releases: []string{},
			current:  "x",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findPreviousRelease(tt.releases, tt.current)
			if (err != nil) != tt.wantErr {
				t.Fatalf("findPreviousRelease() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("findPreviousRelease() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildAppRunCommand_SharedByDeployAndRollback(t *testing.T) {
	// Guards #44: rollback and reload must start the container with the SAME
	// mounts, env and restart policy as deploy — previously rollback only
	// mounted .env.local and lost the managed DATABASE_URL and shared dirs.
	cfg := &config.ProjectConfig{
		Name: "myapp",
		Deploy: config.DeployConfig{
			SharedDirs:  []string{"var/log"},
			SharedFiles: []string{".env.local"},
		},
	}

	cmd := buildAppRunCommand(cfg, "myapp:t1", "/opt/frankendeploy/apps/myapp", "postgres://u:p@myapp-db:5432/db", "myapp-rollback")

	for _, want := range []string{
		"docker run -d --name myapp-rollback",
		"--restart unless-stopped",
		"--user 1000:1000",
		"-e DATABASE_URL=",
		"-v /opt/frankendeploy/apps/myapp/shared/var/log:/app/var/log",
		"-v /opt/frankendeploy/apps/myapp/shared/.env.local:/app/.env.local:ro",
		"myapp:t1",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("buildAppRunCommand() missing %q\ngot: %s", want, cmd)
		}
	}
}

func TestReadSavedDatabaseURL(t *testing.T) {
	t.Run("returns trimmed content", func(t *testing.T) {
		mock := &ssh.MockExecutor{
			ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, ".db_credentials") {
					return &ssh.ExecResult{ExitCode: 0, Stdout: "postgres://u:p@db:5432/app\n"}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
		}
		got := readSavedDatabaseURL(context.Background(), mock, "/opt/frankendeploy/apps/myapp")
		if got != "postgres://u:p@db:5432/app" {
			t.Errorf("readSavedDatabaseURL() = %q", got)
		}
	})

	t.Run("empty when file missing", func(t *testing.T) {
		mock := &ssh.MockExecutor{
			ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{ExitCode: 1}, nil
			},
		}
		if got := readSavedDatabaseURL(context.Background(), mock, "/x"); got != "" {
			t.Errorf("expected empty URL, got %q", got)
		}
	})
}

func TestReloadContainer_UsesDeployPrimitives(t *testing.T) {
	// Guards #44: reload previously omitted --restart unless-stopped (container
	// lost on VPS reboot), shared dirs and the managed DATABASE_URL.
	mock := &ssh.MockExecutor{
		ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
			switch {
			case strings.Contains(command, "{{.Config.Image}}"):
				return &ssh.ExecResult{ExitCode: 0, Stdout: "myapp:t1\n"}, nil
			case strings.Contains(command, ".db_credentials"):
				return &ssh.ExecResult{ExitCode: 0, Stdout: "postgres://u:p@myapp-db:5432/app\n"}, nil
			case strings.Contains(command, "{{.State.Health.Status}}"):
				return &ssh.ExecResult{ExitCode: 0, Stdout: "healthy\n"}, nil
			default:
				return &ssh.ExecResult{ExitCode: 0}, nil
			}
		},
	}
	cfg := &config.ProjectConfig{
		Name: "myapp",
		Deploy: config.DeployConfig{
			SharedDirs:  []string{"var/log"},
			SharedFiles: []string{".env.local"},
		},
	}

	if err := reloadContainer(context.Background(), mock, cfg); err != nil {
		t.Fatalf("reloadContainer() unexpected error: %v", err)
	}

	for _, want := range []string{
		"--restart unless-stopped",
		"-e DATABASE_URL=",
		"/shared/var/log:/app/var/log",
		"docker rename myapp myapp-old",
		"docker rename myapp-new myapp",
	} {
		if !hasCommand(mock.Commands, want) {
			t.Errorf("reloadContainer() missing %q\nrecorded: %v", want, mock.Commands)
		}
	}
}
