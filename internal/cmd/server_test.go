package cmd

import (
	"strings"
	"testing"
)

// joinSetupCommands flattens setup commands for substring assertions.
func joinSetupCommands(cmds []setupCommand) string {
	lines := make([]string, len(cmds))
	for i, c := range cmds {
		lines[i] = c.cmd
	}
	return strings.Join(lines, "\n")
}

func TestBuildFirewallCommands_CustomSSHPort(t *testing.T) {
	cmds := buildFirewallCommands([]int{3022})

	joined := joinSetupCommands(cmds)
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

	joined := joinSetupCommands(cmds)
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

	joined := joinSetupCommands(cmds)
	for _, want := range []string{"allow 80/tcp", "allow 443/tcp"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected %q in firewall commands, got:\n%s", want, joined)
		}
	}
	// The enable must come last: every allow runs before the firewall goes up.
	last := cmds[len(cmds)-1]
	if !strings.Contains(last.cmd, "ufw --force enable") {
		t.Errorf("expected 'ufw --force enable' to be the last command, got %q", last.cmd)
	}
	for _, cmd := range cmds[:len(cmds)-1] {
		if strings.Contains(cmd.cmd, "enable") {
			t.Errorf("enable found before the last position: %q", cmd.cmd)
		}
	}
}

func TestBuildFirewallCommands_InvalidPortsFiltered(t *testing.T) {
	// A failed detection (0) must not produce an 'allow 0/tcp' rule, and at
	// least the valid port must still be allowed before enabling.
	cmds := buildFirewallCommands([]int{0, -1, 2222})

	joined := joinSetupCommands(cmds)
	if strings.Contains(joined, "allow 0/tcp") || strings.Contains(joined, "allow -1/tcp") {
		t.Errorf("invalid ports must be filtered, got:\n%s", joined)
	}
	if !strings.Contains(joined, "allow 2222/tcp") {
		t.Errorf("expected valid port 2222 to be allowed, got:\n%s", joined)
	}
}

// --- Issue #62: OS detection and explicit failures ---

func TestCheckOSSupported(t *testing.T) {
	tests := []struct {
		name      string
		osRelease string
		wantErr   bool
	}{
		{"ubuntu", "ID=ubuntu\nID_LIKE=debian\nPRETTY_NAME=\"Ubuntu 24.04.3 LTS\"\n", false},
		{"debian", "ID=debian\nPRETTY_NAME=\"Debian GNU/Linux 12\"\n", false},
		{"raspbian (ID_LIKE)", "ID=raspbian\nID_LIKE=debian\n", false},
		{"linuxmint (ID_LIKE multi)", "ID=linuxmint\nID_LIKE=\"ubuntu debian\"\n", false},
		{"rocky", "ID=\"rocky\"\nID_LIKE=\"rhel centos fedora\"\nPRETTY_NAME=\"Rocky Linux 9\"\n", true},
		{"fedora", "ID=fedora\n", true},
		{"arch", "ID=arch\n", true},
		{"empty", "", true},
	}
	for _, tt := range tests {
		err := checkOSSupported(tt.osRelease)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: checkOSSupported() error = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
		if err != nil && !strings.Contains(err.Error(), "Ubuntu") {
			t.Errorf("%s: the error must list supported distributions, got: %v", tt.name, err)
		}
	}
}

func TestValidateSetupEmail(t *testing.T) {
	for _, valid := range []string{"user@example.com", "first.last+tag@sub.domain.org"} {
		if err := validateSetupEmail(valid); err != nil {
			t.Errorf("validateSetupEmail(%q) unexpected error: %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "not-an-email", "user@", "@example.com", "user @example.com"} {
		if err := validateSetupEmail(invalid); err == nil {
			t.Errorf("validateSetupEmail(%q) must fail (a typo only surfaces at the Let's Encrypt failure otherwise)", invalid)
		}
	}
}
