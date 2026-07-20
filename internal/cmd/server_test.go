package cmd

import (
	"strings"
	"testing"
)

func TestBuildFirewallCommands_CustomSSHPort(t *testing.T) {
	cmds := buildFirewallCommands([]int{3022})

	joined := strings.Join(cmds, "\n")
	if !strings.Contains(joined, "sudo ufw allow 3022/tcp") {
		t.Errorf("expected the configured SSH port 3022 to be allowed, got:\n%s", joined)
	}
	if strings.Contains(joined, "allow 22/tcp") {
		t.Errorf("port 22 must not be hardcoded when SSH uses another port, got:\n%s", joined)
	}
}

func TestBuildFirewallCommands_MultiplePortsDeduplicated(t *testing.T) {
	// Configured port + server-side detected port (gateway/NAT case), with a duplicate
	cmds := buildFirewallCommands([]int{3022, 22, 3022})

	joined := strings.Join(cmds, "\n")
	if !strings.Contains(joined, "allow 3022/tcp") || !strings.Contains(joined, "allow 22/tcp") {
		t.Errorf("expected both SSH ports to be allowed, got:\n%s", joined)
	}
	count := strings.Count(joined, "allow 3022/tcp")
	if count != 1 {
		t.Errorf("expected port 3022 to be allowed exactly once, got %d times", count)
	}
}

func TestBuildFirewallCommands_HTTPPortsAndEnableLast(t *testing.T) {
	cmds := buildFirewallCommands([]int{22})

	joined := strings.Join(cmds, "\n")
	for _, want := range []string{"allow 80/tcp", "allow 443/tcp"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected %q in firewall commands, got:\n%s", want, joined)
		}
	}
	// The enable must come last: every allow runs before the firewall goes up.
	last := cmds[len(cmds)-1]
	if !strings.Contains(last, "ufw --force enable") {
		t.Errorf("expected 'ufw --force enable' to be the last command, got %q", last)
	}
	for _, cmd := range cmds[:len(cmds)-1] {
		if strings.Contains(cmd, "enable") {
			t.Errorf("enable found before the last position: %q", cmd)
		}
	}
}

func TestBuildFirewallCommands_InvalidPortsFiltered(t *testing.T) {
	// A failed detection (0) must not produce an 'allow 0/tcp' rule, and at
	// least the valid port must still be allowed before enabling.
	cmds := buildFirewallCommands([]int{0, -1, 2222})

	joined := strings.Join(cmds, "\n")
	if strings.Contains(joined, "allow 0/tcp") || strings.Contains(joined, "allow -1/tcp") {
		t.Errorf("invalid ports must be filtered, got:\n%s", joined)
	}
	if !strings.Contains(joined, "allow 2222/tcp") {
		t.Errorf("expected valid port 2222 to be allowed, got:\n%s", joined)
	}
}
