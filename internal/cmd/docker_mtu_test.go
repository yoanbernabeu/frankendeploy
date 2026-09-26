package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

// fakeMTUHost simulates a server for the MTU helpers.
type fakeMTUHost struct {
	hostMTU      string // "" when undetectable
	bridgeMTU    string
	daemonJSON   string // "" when absent
	networkMTU   string // option value; "missing" when the network does not exist
	networkInUse bool   // docker network rm fails
}

func (f *fakeMTUHost) executor() *ssh.MockExecutor {
	mock := &ssh.MockExecutor{}
	mock.ExecFunc = func(ctx context.Context, command string) (*ssh.ExecResult, error) {
		ok := func(out string) (*ssh.ExecResult, error) { return &ssh.ExecResult{Stdout: out}, nil }
		fail := func() (*ssh.ExecResult, error) { return &ssh.ExecResult{ExitCode: 1}, nil }
		switch {
		case command == hostMTUCommand:
			if f.hostMTU == "" {
				return fail()
			}
			return ok(f.hostMTU + "\n")
		case strings.Contains(command, "docker0/mtu"):
			return ok(f.bridgeMTU + "\n")
		case strings.HasPrefix(command, "cat "+dockerDaemonJSON):
			if f.daemonJSON == "" {
				return fail()
			}
			return ok(f.daemonJSON + "\n")
		case strings.HasPrefix(command, "docker network inspect"):
			if f.networkMTU == "missing" {
				return fail()
			}
			return ok(f.networkMTU + "\n")
		case strings.HasPrefix(command, "docker network rm"):
			if f.networkInUse {
				return fail()
			}
			f.networkMTU = "missing"
			return ok("")
		}
		return ok("")
	}
	return mock
}

func commandsContaining(mock *ssh.MockExecutor, substr string) []string {
	var found []string
	for _, c := range mock.Commands {
		if strings.Contains(c, substr) {
			found = append(found, c)
		}
	}
	return found
}

func TestConfigureDockerMTU_StandardMTU(t *testing.T) {
	for _, hostMTU := range []string{"1500", "9001", ""} {
		mock := (&fakeMTUHost{hostMTU: hostMTU}).executor()

		mtu, err := configureDockerMTU(context.Background(), mock)
		if err != nil || mtu != 0 {
			t.Errorf("host MTU %q: got (%d, %v), want (0, nil)", hostMTU, mtu, err)
		}
		if len(commandsContaining(mock, dockerDaemonJSON)) != 0 {
			t.Errorf("host MTU %q: daemon.json must not be touched", hostMTU)
		}
	}
}

func TestConfigureDockerMTU_WritesDaemonJSON(t *testing.T) {
	mock := (&fakeMTUHost{hostMTU: "1400"}).executor()

	mtu, err := configureDockerMTU(context.Background(), mock)
	if err != nil {
		t.Fatalf("configureDockerMTU() error = %v", err)
	}
	if mtu != 1400 {
		t.Errorf("mtu = %d, want 1400", mtu)
	}
	writes := commandsContaining(mock, "sudo tee "+dockerDaemonJSON)
	if len(writes) != 1 || !strings.Contains(writes[0], dockerDaemonConfig(1400)) {
		t.Errorf("expected daemon.json to be written with %s, got %v", dockerDaemonConfig(1400), writes)
	}
	if len(commandsContaining(mock, "systemctl restart docker")) != 1 {
		t.Error("expected Docker to be restarted once")
	}
}

func TestConfigureDockerMTU_AlreadyConfigured(t *testing.T) {
	mock := (&fakeMTUHost{hostMTU: "1400", daemonJSON: dockerDaemonConfig(1400)}).executor()

	mtu, err := configureDockerMTU(context.Background(), mock)
	if err != nil || mtu != 1400 {
		t.Fatalf("got (%d, %v), want (1400, nil)", mtu, err)
	}
	if len(commandsContaining(mock, "systemctl restart docker")) != 0 {
		t.Error("a re-run must not restart Docker")
	}
}

func TestConfigureDockerMTU_ForeignDaemonJSONUntouched(t *testing.T) {
	mock := (&fakeMTUHost{hostMTU: "1460", daemonJSON: `{"log-driver": "journald"}`}).executor()

	mtu, err := configureDockerMTU(context.Background(), mock)
	if err != nil || mtu != 1460 {
		t.Fatalf("got (%d, %v), want (1460, nil)", mtu, err)
	}
	if len(commandsContaining(mock, "sudo tee")) != 0 || len(commandsContaining(mock, "systemctl restart")) != 0 {
		t.Error("an existing daemon.json must never be modified")
	}
}

func TestEnsureSharedNetwork_CreatesWithMTU(t *testing.T) {
	mock := (&fakeMTUHost{networkMTU: "missing"}).executor()

	if err := ensureSharedNetwork(context.Background(), mock, 1400); err != nil {
		t.Fatalf("ensureSharedNetwork() error = %v", err)
	}
	creates := commandsContaining(mock, "docker network create")
	if len(creates) != 1 || !strings.Contains(creates[0], mtuNetworkOption+"=1400") {
		t.Errorf("expected creation with MTU 1400, got %v", creates)
	}
}

func TestEnsureSharedNetwork_StandardMTU(t *testing.T) {
	mock := (&fakeMTUHost{networkMTU: "missing"}).executor()

	if err := ensureSharedNetwork(context.Background(), mock, 0); err != nil {
		t.Fatalf("ensureSharedNetwork() error = %v", err)
	}
	creates := commandsContaining(mock, "docker network create")
	if len(creates) != 1 || strings.Contains(creates[0], mtuNetworkOption) {
		t.Errorf("expected a plain creation, got %v", creates)
	}
}

func TestEnsureSharedNetwork_ExistingKept(t *testing.T) {
	mock := (&fakeMTUHost{networkMTU: "1400"}).executor()

	if err := ensureSharedNetwork(context.Background(), mock, 1400); err != nil {
		t.Fatalf("ensureSharedNetwork() error = %v", err)
	}
	if len(commandsContaining(mock, "docker network rm")) != 0 || len(commandsContaining(mock, "docker network create")) != 0 {
		t.Errorf("an aligned network must be left alone, got %v", mock.Commands)
	}
}

func TestEnsureSharedNetwork_RecreatesLargerMTU(t *testing.T) {
	mock := (&fakeMTUHost{networkMTU: ""}).executor() // no option: 1500

	if err := ensureSharedNetwork(context.Background(), mock, 1400); err != nil {
		t.Fatalf("ensureSharedNetwork() error = %v", err)
	}
	if len(commandsContaining(mock, "docker rm -f caddy")) != 1 {
		t.Error("expected Caddy to be removed before the network")
	}
	creates := commandsContaining(mock, "docker network create")
	if len(creates) != 1 || !strings.Contains(creates[0], mtuNetworkOption+"=1400") {
		t.Errorf("expected the network to be recreated with MTU 1400, got %v", creates)
	}
}

func TestEnsureSharedNetwork_InUseKept(t *testing.T) {
	mock := (&fakeMTUHost{networkMTU: "", networkInUse: true}).executor()

	if err := ensureSharedNetwork(context.Background(), mock, 1400); err != nil {
		t.Fatalf("a network in use must not fail the setup: %v", err)
	}
	if len(commandsContaining(mock, "docker network create")) != 0 {
		t.Error("a network in use must not be recreated")
	}
}

func TestCheckNetworkMTU(t *testing.T) {
	tests := []struct {
		name    string
		host    fakeMTUHost
		wantOK  bool
		wantMsg string
	}{
		{"standard MTU", fakeMTUHost{hostMTU: "1500"}, true, "1500"},
		{"undetectable", fakeMTUHost{}, true, "could not determine"},
		{"aligned", fakeMTUHost{hostMTU: "1400", bridgeMTU: "1400", networkMTU: "1400"}, true, "aligned"},
		{"bridge too large", fakeMTUHost{hostMTU: "1400", bridgeMTU: "1500", networkMTU: "1400"}, false, "docker0 (builds) uses 1500"},
		{"network too large", fakeMTUHost{hostMTU: "1460", bridgeMTU: "1460", networkMTU: ""}, false, "frankendeploy uses 1500"},
		{"network missing", fakeMTUHost{hostMTU: "1400", bridgeMTU: "1400", networkMTU: "missing"}, true, "aligned"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checkNetworkMTU(context.Background(), tt.host.executor())
			if result.OK != tt.wantOK {
				t.Errorf("OK = %v, want %v (%s)", result.OK, tt.wantOK, result.Detail)
			}
			if !strings.Contains(result.Detail, tt.wantMsg) {
				t.Errorf("Detail = %q, want it to contain %q", result.Detail, tt.wantMsg)
			}
			if !tt.wantOK && !strings.Contains(result.Advice, "server setup") {
				t.Errorf("Advice should point to server setup, got %q", result.Advice)
			}
		})
	}
}
