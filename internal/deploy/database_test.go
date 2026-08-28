package deploy

import (
	"context"
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

func managedPgTestConfig() *config.ProjectConfig {
	managed := true
	cfg := &config.ProjectConfig{Name: "myapp"}
	cfg.Database = config.DatabaseConfig{Driver: "pgsql", Version: "16", Managed: &managed}
	return cfg
}

const savedTestURL = "postgresql://myapp:oldpass@myapp-db:5432/myapp?serverVersion=16&charset=utf8"

// dbMock builds a MockExecutor scripted per command pattern.
func dbMock(responses map[string]ssh.ExecResult) *ssh.MockExecutor {
	return &ssh.MockExecutor{
		ExecFunc: func(_ context.Context, command string) (*ssh.ExecResult, error) {
			for pattern, result := range responses {
				if strings.Contains(command, pattern) {
					r := result
					return &r, nil
				}
			}
			return &ssh.ExecResult{ExitCode: 0}, nil
		},
	}
}

func TestDeployManagedDatabase_ReusesRunningContainerCredentials(t *testing.T) {
	mock := dbMock(map[string]ssh.ExecResult{
		"cat /opt/frankendeploy/apps/myapp/shared/.db_credentials": {Stdout: savedTestURL + "\n", ExitCode: 0},
		"docker ps -aq": {Stdout: "abc123\n", ExitCode: 0},
		"docker ps -q":  {Stdout: "abc123\n", ExitCode: 0},
	})

	url, err := DeployManagedDatabase(context.Background(), mock, managedPgTestConfig(), "/opt/frankendeploy/apps/myapp", nil)
	if err != nil {
		t.Fatalf("DeployManagedDatabase: %v", err)
	}
	if url != savedTestURL {
		t.Errorf("expected saved URL to be reused, got %q", url)
	}
	for _, cmd := range mock.Commands {
		if strings.Contains(cmd, "docker run") {
			t.Errorf("must not recreate a running container with saved credentials: %s", cmd)
		}
	}
}

func TestDeployManagedDatabase_StartsStoppedContainerAndReusesCredentials(t *testing.T) {
	// VPS rebooted: container exists but is NOT running. The old code only
	// checked `docker ps -q`, regenerated a password the datadir ignores,
	// and saved a wrong URL.
	mock := dbMock(map[string]ssh.ExecResult{
		"cat /opt/frankendeploy/apps/myapp/shared/.db_credentials": {Stdout: savedTestURL + "\n", ExitCode: 0},
		"docker ps -aq":  {Stdout: "abc123\n", ExitCode: 0},
		"docker ps -q -": {Stdout: "", ExitCode: 0}, // not running
		"pg_isready":     {ExitCode: 0},
	})

	url, err := DeployManagedDatabase(context.Background(), mock, managedPgTestConfig(), "/opt/frankendeploy/apps/myapp", nil)
	if err != nil {
		t.Fatalf("DeployManagedDatabase: %v", err)
	}
	if url != savedTestURL {
		t.Errorf("expected saved URL, got %q", url)
	}
	started := false
	for _, cmd := range mock.Commands {
		if strings.Contains(cmd, "docker start myapp-db") {
			started = true
		}
		if strings.Contains(cmd, "docker run") {
			t.Errorf("must docker start, not recreate: %s", cmd)
		}
	}
	if !started {
		t.Error("expected the stopped container to be started")
	}
}

func TestDeployManagedDatabase_VolumeWithoutCredentialsFailsExplicitly(t *testing.T) {
	mock := dbMock(map[string]ssh.ExecResult{
		"cat /opt/frankendeploy/apps/myapp/shared/.db_credentials": {ExitCode: 1},
		"docker ps -aq":    {Stdout: "", ExitCode: 0},
		"docker volume ls": {Stdout: "myapp-db-data\n", ExitCode: 0},
	})

	_, err := DeployManagedDatabase(context.Background(), mock, managedPgTestConfig(), "/opt/frankendeploy/apps/myapp", nil)
	if err == nil {
		t.Fatal("a data volume without credentials must fail explicitly (regenerated password would be ignored by the datadir)")
	}
	if !strings.Contains(err.Error(), "docker volume rm") {
		t.Errorf("the error must explain how to recover, got: %v", err)
	}
	for _, cmd := range mock.Commands {
		if strings.Contains(cmd, "docker run") {
			t.Errorf("must not create a container over an orphaned volume: %s", cmd)
		}
	}
}

func TestDeployManagedDatabase_FreshSetupCreatesAndSaves(t *testing.T) {
	mock := dbMock(map[string]ssh.ExecResult{
		"cat /opt/frankendeploy/apps/myapp/shared/.db_credentials": {ExitCode: 1},
		"docker ps -aq":    {Stdout: "", ExitCode: 0},
		"docker volume ls": {Stdout: "", ExitCode: 0},
		"pg_isready":       {ExitCode: 0},
	})

	url, err := DeployManagedDatabase(context.Background(), mock, managedPgTestConfig(), "/opt/frankendeploy/apps/myapp", nil)
	if err != nil {
		t.Fatalf("DeployManagedDatabase: %v", err)
	}
	if !strings.HasPrefix(url, "postgresql://myapp:") {
		t.Errorf("expected a fresh postgresql URL, got %q", url)
	}
	var ran, saved bool
	for _, cmd := range mock.Commands {
		if strings.Contains(cmd, "docker run") && strings.Contains(cmd, "postgres:16-alpine") {
			ran = true
		}
		if strings.Contains(cmd, ".db_credentials") && strings.HasPrefix(cmd, "echo ") {
			saved = true
		}
	}
	if !ran {
		t.Error("expected a docker run with the postgres image")
	}
	if !saved {
		t.Error("expected credentials to be saved")
	}
}

func TestDeployManagedDatabase_ReadinessTimeoutFails(t *testing.T) {
	old := DBReadinessAttempts
	DBReadinessAttempts = 2
	defer func() { DBReadinessAttempts = old }()

	mock := dbMock(map[string]ssh.ExecResult{
		"cat /opt/frankendeploy/apps/myapp/shared/.db_credentials": {ExitCode: 1},
		"docker ps -aq":    {Stdout: "", ExitCode: 0},
		"docker volume ls": {Stdout: "", ExitCode: 0},
		"pg_isready":       {ExitCode: 1}, // never ready
	})

	_, err := DeployManagedDatabase(context.Background(), mock, managedPgTestConfig(), "/opt/frankendeploy/apps/myapp", nil)
	if err == nil {
		t.Fatal("a database that never becomes ready must fail the deploy (the silent continue exploded later in migrations)")
	}
	if !strings.Contains(err.Error(), "docker logs") {
		t.Errorf("the error must point at the container logs, got: %v", err)
	}
}
