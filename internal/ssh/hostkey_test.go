package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// testHostKey generates an ed25519 host key pair for tests
func testHostKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("failed to convert key: %v", err)
	}
	return sshPub
}

type fakeAddr struct{ addr string }

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return a.addr }

func TestKnownHostsCallbackWithTOFU_UnknownHostAccepted(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "known_hosts")
	key := testHostKey(t)

	var promptedHost, promptedFingerprint string
	prompt := func(host, keyType, fingerprint string) bool {
		promptedHost = host
		promptedFingerprint = fingerprint
		return true
	}

	callback, err := knownHostsCallbackWithTOFU(path, prompt)
	if err != nil {
		t.Fatalf("knownHostsCallbackWithTOFU() error = %v", err)
	}

	addr := fakeAddr{"192.0.2.1:22"}
	if err := callback("example.com:22", addr, key); err != nil {
		t.Fatalf("expected TOFU acceptance, got error: %v", err)
	}

	if promptedHost != "example.com" {
		t.Errorf("prompted host = %q, want %q", promptedHost, "example.com")
	}
	if promptedFingerprint != ssh.FingerprintSHA256(key) {
		t.Errorf("prompted fingerprint = %q, want %q", promptedFingerprint, ssh.FingerprintSHA256(key))
	}

	// The key must have been appended to known_hosts
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read known_hosts: %v", err)
	}
	if !strings.Contains(string(data), "example.com") {
		t.Errorf("known_hosts does not contain the accepted host: %s", data)
	}

	// A fresh callback (re-reading the file) must now accept without prompting
	prompted := false
	callback2, err := knownHostsCallbackWithTOFU(path, func(host, keyType, fingerprint string) bool {
		prompted = true
		return false
	})
	if err != nil {
		t.Fatalf("knownHostsCallbackWithTOFU() error = %v", err)
	}
	if err := callback2("example.com:22", addr, key); err != nil {
		t.Errorf("expected known host to be accepted, got: %v", err)
	}
	if prompted {
		t.Error("prompt was called for an already-known host")
	}
}

func TestKnownHostsCallbackWithTOFU_UnknownHostRejected(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "known_hosts")
	key := testHostKey(t)

	callback, err := knownHostsCallbackWithTOFU(path, func(host, keyType, fingerprint string) bool {
		return false
	})
	if err != nil {
		t.Fatalf("knownHostsCallbackWithTOFU() error = %v", err)
	}

	err = callback("example.com:22", fakeAddr{"192.0.2.1:22"}, key)
	var unknownErr *HostKeyUnknownError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("expected HostKeyUnknownError, got: %v", err)
	}

	// Nothing must have been written
	if data, _ := os.ReadFile(path); strings.Contains(string(data), "example.com") {
		t.Error("rejected host was written to known_hosts")
	}
}

func TestKnownHostsCallbackWithTOFU_NilPromptRejects(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "known_hosts")
	key := testHostKey(t)

	callback, err := knownHostsCallbackWithTOFU(path, nil)
	if err != nil {
		t.Fatalf("knownHostsCallbackWithTOFU() error = %v", err)
	}

	err = callback("example.com:22", fakeAddr{"192.0.2.1:22"}, key)
	var unknownErr *HostKeyUnknownError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("expected HostKeyUnknownError with nil prompt, got: %v", err)
	}
}

func TestKnownHostsCallbackWithTOFU_ChangedKey(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "known_hosts")
	oldKey := testHostKey(t)
	newKey := testHostKey(t)

	// Record the old key
	line := knownhosts.Line([]string{"example.com:22"}, oldKey)
	if err := os.WriteFile(path, []byte(line+"\n"), 0600); err != nil {
		t.Fatalf("failed to write known_hosts: %v", err)
	}

	prompted := false
	callback, err := knownHostsCallbackWithTOFU(path, func(host, keyType, fingerprint string) bool {
		prompted = true
		return true
	})
	if err != nil {
		t.Fatalf("knownHostsCallbackWithTOFU() error = %v", err)
	}

	err = callback("example.com:22", fakeAddr{"192.0.2.1:22"}, newKey)
	var changedErr *HostKeyChangedError
	if !errors.As(err, &changedErr) {
		t.Fatalf("expected HostKeyChangedError, got: %v", err)
	}
	if prompted {
		t.Error("TOFU prompt must never be offered on a host key mismatch")
	}
	if !strings.Contains(changedErr.Error(), "ssh-keygen -R") {
		t.Errorf("error message should mention ssh-keygen -R, got: %v", changedErr)
	}
}

func TestKnownHostsCallbackWithTOFU_CreatesMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, ".ssh", "known_hosts")

	if _, err := knownHostsCallbackWithTOFU(path, nil); err != nil {
		t.Fatalf("expected missing known_hosts to be created, got error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("known_hosts was not created: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("known_hosts permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestKnownHostsCallbackWithTOFU_NonStandardPort(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "known_hosts")
	key := testHostKey(t)

	callback, err := knownHostsCallbackWithTOFU(path, func(host, keyType, fingerprint string) bool {
		return true
	})
	if err != nil {
		t.Fatalf("knownHostsCallbackWithTOFU() error = %v", err)
	}

	addr := fakeAddr{"192.0.2.1:3022"}
	if err := callback("gate.example.com:3022", addr, key); err != nil {
		t.Fatalf("expected TOFU acceptance, got error: %v", err)
	}

	// A fresh callback must recognize the host on its non-standard port
	callback2, err := knownHostsCallbackWithTOFU(path, nil)
	if err != nil {
		t.Fatalf("knownHostsCallbackWithTOFU() error = %v", err)
	}
	if err := callback2("gate.example.com:3022", addr, key); err != nil {
		t.Errorf("host on non-standard port not recognized after TOFU: %v", err)
	}
}

func TestResolveHostKeyCallback_EnvKnownHosts(t *testing.T) {
	key := testHostKey(t)
	line := knownhosts.Line([]string{"example.com:22"}, key)
	t.Setenv("FRANKENDEPLOY_KNOWN_HOSTS", line+"\n")

	callback, err := ResolveHostKeyCallback(nil)
	if err != nil {
		t.Fatalf("ResolveHostKeyCallback() error = %v", err)
	}

	if err := callback("example.com:22", fakeAddr{"192.0.2.1:22"}, key); err != nil {
		t.Errorf("host from FRANKENDEPLOY_KNOWN_HOSTS rejected: %v", err)
	}

	// An unknown host must be rejected (no TOFU on env-provided known_hosts)
	otherKey := testHostKey(t)
	if err := callback("other.com:22", fakeAddr{"192.0.2.2:22"}, otherKey); err == nil {
		t.Error("unknown host accepted with FRANKENDEPLOY_KNOWN_HOSTS set")
	}
}

func TestResolveHostKeyCallback_SkipCheck(t *testing.T) {
	t.Setenv("FRANKENDEPLOY_SKIP_HOST_KEY_CHECK", "true")

	callback, err := ResolveHostKeyCallback(nil)
	if err != nil {
		t.Fatalf("ResolveHostKeyCallback() error = %v", err)
	}

	if err := callback("anything.com:22", fakeAddr{"192.0.2.1:22"}, testHostKey(t)); err != nil {
		t.Errorf("expected host to be accepted with skip check, got: %v", err)
	}
}

func TestHostKeyChangedError_Message(t *testing.T) {
	err := &HostKeyChangedError{Host: "example.com", Fingerprint: "SHA256:abc"}
	msg := err.Error()
	for _, want := range []string{"example.com", "SHA256:abc", "ssh-keygen -R", "man-in-the-middle"} {
		if !strings.Contains(msg, want) {
			t.Errorf("HostKeyChangedError message missing %q: %s", want, msg)
		}
	}
}

func TestHostKeyUnknownError_Message(t *testing.T) {
	err := &HostKeyUnknownError{Host: "example.com", Fingerprint: "SHA256:abc"}
	msg := err.Error()
	for _, want := range []string{"example.com", "SHA256:abc", "FRANKENDEPLOY_KNOWN_HOSTS"} {
		if !strings.Contains(msg, want) {
			t.Errorf("HostKeyUnknownError message missing %q: %s", want, msg)
		}
	}
}

// Guard against net import being reported unused if tests change
var _ net.Addr = fakeAddr{}
