package deploy

import (
	"context"
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

// networkMock scripts docker answers per command substring; unmatched
// commands succeed silently.
func networkMock(responses map[string]ssh.ExecResult) *ssh.MockExecutor {
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

func hasExactCommand(commands []string, want string) bool {
	for _, c := range commands {
		if c == want {
			return true
		}
	}
	return false
}

func hasCommand(commands []string, fragment string) bool {
	for _, c := range commands {
		if strings.Contains(c, fragment) {
			return true
		}
	}
	return false
}

func TestEnsureAppNetwork_CreatesNetworkAndAttachesCaddy(t *testing.T) {
	mock := networkMock(map[string]ssh.ExecResult{
		"docker network inspect frankendeploy-myapp": {ExitCode: 1},
		"docker inspect caddy":                       {Stdout: "frankendeploy \n"},
		"docker inspect myapp-db":                    {ExitCode: 1}, // no managed database
	})

	if err := EnsureAppNetwork(context.Background(), mock, "myapp", nil); err != nil {
		t.Fatalf("EnsureAppNetwork: %v", err)
	}
	if !hasCommand(mock.Commands, "docker network create --label frankendeploy.app=myapp frankendeploy-myapp") {
		t.Errorf("expected the app network to be created, got %v", mock.Commands)
	}
	if !hasCommand(mock.Commands, "docker network connect frankendeploy-myapp caddy") {
		t.Errorf("Caddy must be attached to the app network, got %v", mock.Commands)
	}
	if hasCommand(mock.Commands, "docker network connect frankendeploy-myapp myapp-db") {
		t.Errorf("no database container: nothing to attach, got %v", mock.Commands)
	}
}

func TestEnsureAppNetwork_IsIdempotent(t *testing.T) {
	mock := networkMock(map[string]ssh.ExecResult{
		"docker network inspect frankendeploy-myapp": {Stdout: "frankendeploy-myapp\n"},
		"docker inspect caddy":                       {Stdout: "frankendeploy frankendeploy-myapp \n"},
		"docker inspect myapp-db":                    {Stdout: "frankendeploy-myapp \n"},
	})

	if err := EnsureAppNetwork(context.Background(), mock, "myapp", nil); err != nil {
		t.Fatalf("EnsureAppNetwork: %v", err)
	}
	for _, forbidden := range []string{"docker network create", "docker network connect"} {
		if hasCommand(mock.Commands, forbidden) {
			t.Errorf("second run must change nothing, but ran %q: %v", forbidden, mock.Commands)
		}
	}
}

func TestEnsureAppNetwork_MigratesExistingDatabase(t *testing.T) {
	// Database created before per-app networks: attached to the shared
	// network only (even when stopped, docker inspect still lists it).
	mock := networkMock(map[string]ssh.ExecResult{
		"docker network inspect frankendeploy-myapp": {Stdout: "frankendeploy-myapp\n"},
		"docker inspect caddy":                       {Stdout: "frankendeploy frankendeploy-myapp \n"},
		"docker inspect myapp-db":                    {Stdout: "frankendeploy \n"},
	})

	if err := EnsureAppNetwork(context.Background(), mock, "myapp", nil); err != nil {
		t.Fatalf("EnsureAppNetwork: %v", err)
	}
	if !hasCommand(mock.Commands, "docker network connect frankendeploy-myapp myapp-db") {
		t.Errorf("the existing database must join the app network, got %v", mock.Commands)
	}
	if hasCommand(mock.Commands, "docker network disconnect") {
		t.Errorf("the database must stay on the shared network until the old app container is gone, got %v", mock.Commands)
	}
}

func TestEnsureAppNetwork_CaddyConnectFailureIsFatal(t *testing.T) {
	mock := networkMock(map[string]ssh.ExecResult{
		"docker network inspect frankendeploy-myapp":       {Stdout: "frankendeploy-myapp\n"},
		"docker inspect caddy":                             {Stdout: "frankendeploy \n"},
		"docker network connect frankendeploy-myapp caddy": {ExitCode: 1, Stderr: "Error response from daemon: boom"},
	})

	err := EnsureAppNetwork(context.Background(), mock, "myapp", nil)
	if err == nil || !strings.Contains(err.Error(), "Caddy") {
		t.Fatalf("a proxy that cannot join the network must fail the deploy, got %v", err)
	}
}

func TestEnsureAppNetwork_ExplainsAddressPoolExhaustion(t *testing.T) {
	mock := networkMock(map[string]ssh.ExecResult{
		"docker network inspect frankendeploy-myapp": {ExitCode: 1},
		"docker network create":                      {ExitCode: 1, Stderr: "Error response from daemon: could not find an available, non-overlapping IPv4 address pool among the defaults to assign to the network"},
	})

	err := EnsureAppNetwork(context.Background(), mock, "myapp", nil)
	if err == nil || !strings.Contains(err.Error(), "default-address-pools") {
		t.Fatalf("pool exhaustion must explain the daemon.json fix, got %v", err)
	}
}

func TestDetachFromSharedNetwork_OnlyWhenOnBothNetworks(t *testing.T) {
	mock := networkMock(map[string]ssh.ExecResult{
		"docker inspect myapp ":       {Stdout: "frankendeploy-myapp \n"},               // already migrated
		"docker inspect myapp-worker": {ExitCode: 1},                                    // no worker
		"docker inspect myapp-db":     {Stdout: "frankendeploy frankendeploy-myapp \n"}, // mid-migration
	})

	DetachFromSharedNetwork(context.Background(), mock, "myapp", nil)

	if !hasCommand(mock.Commands, "docker network disconnect frankendeploy myapp-db") {
		t.Errorf("the database must leave the shared network once on the app network, got %v", mock.Commands)
	}
	if hasExactCommand(mock.Commands, "docker network disconnect frankendeploy myapp") || hasCommand(mock.Commands, "docker network disconnect frankendeploy myapp-worker") {
		t.Errorf("containers not on the shared network must be left alone, got %v", mock.Commands)
	}
}

func TestDetachFromSharedNetwork_NeverLeavesContainerWithoutNetwork(t *testing.T) {
	mock := networkMock(map[string]ssh.ExecResult{
		"docker inspect myapp-db": {Stdout: "frankendeploy \n"}, // not yet on the app network
	})

	DetachFromSharedNetwork(context.Background(), mock, "myapp", nil)

	if hasCommand(mock.Commands, "docker network disconnect") {
		t.Errorf("a container only on the shared network must not be disconnected, got %v", mock.Commands)
	}
}

func TestRemoveAppNetwork(t *testing.T) {
	mock := networkMock(map[string]ssh.ExecResult{
		"docker network inspect frankendeploy-myapp": {Stdout: "frankendeploy-myapp\n"},
	})

	if err := RemoveAppNetwork(context.Background(), mock, "myapp"); err != nil {
		t.Fatalf("RemoveAppNetwork: %v", err)
	}
	if !hasCommand(mock.Commands, "docker network disconnect frankendeploy-myapp caddy") {
		t.Errorf("Caddy must be detached before removing the network, got %v", mock.Commands)
	}
	if !hasCommand(mock.Commands, "docker network rm frankendeploy-myapp") {
		t.Errorf("the app network must be removed, got %v", mock.Commands)
	}
}
