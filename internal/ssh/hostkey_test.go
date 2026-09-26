package ssh

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
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

func TestResolveHostKey_EnvKnownHosts(t *testing.T) {
	key := testHostKey(t)
	line := knownhosts.Line([]string{"example.com:22"}, key)
	t.Setenv("FRANKENDEPLOY_KNOWN_HOSTS", line+"\n")

	callback, _, err := ResolveHostKey(nil, "example.com:22")
	if err != nil {
		t.Fatalf("ResolveHostKey() error = %v", err)
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

func TestResolveHostKey_SkipCheck(t *testing.T) {
	t.Setenv("FRANKENDEPLOY_SKIP_HOST_KEY_CHECK", "true")

	callback, _, err := ResolveHostKey(nil, "example.com:22")
	if err != nil {
		t.Fatalf("ResolveHostKey() error = %v", err)
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

// knownHostsWith returns a known_hosts callback holding the given keys for host.
func knownHostsWith(t *testing.T, host string, keys ...ssh.PublicKey) ssh.HostKeyCallback {
	t.Helper()
	var lines []string
	for _, key := range keys {
		lines = append(lines, knownhosts.Line([]string{host}, key))
	}
	path := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatalf("failed to write known_hosts: %v", err)
	}
	callback, err := knownhosts.New(path)
	if err != nil {
		t.Fatalf("knownhosts.New() error = %v", err)
	}
	return callback
}

func testSigner(t *testing.T, generate func() (crypto.Signer, error)) ssh.Signer {
	t.Helper()
	key, err := generate()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromSigner(key)
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}
	return signer
}

func ed25519Signer(t *testing.T) ssh.Signer {
	return testSigner(t, func() (crypto.Signer, error) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		return priv, err
	})
}

func ecdsaSigner(t *testing.T) ssh.Signer {
	return testSigner(t, func() (crypto.Signer, error) {
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	})
}

func TestPreferredHostKeyAlgorithms_UnknownHost(t *testing.T) {
	known := knownHostsWith(t, "other.example.com:22", ed25519Signer(t).PublicKey())

	if algos := preferredHostKeyAlgorithms(known, "example.com:22"); algos != nil {
		t.Errorf("expected nil for an unknown host, got %v", algos)
	}
}

func TestPreferredHostKeyAlgorithms_KnownTypeFirst(t *testing.T) {
	known := knownHostsWith(t, "example.com:22", ed25519Signer(t).PublicKey())

	algos := preferredHostKeyAlgorithms(known, "example.com:22")
	if len(algos) == 0 || algos[0] != ssh.KeyAlgoED25519 {
		t.Fatalf("expected %s first, got %v", ssh.KeyAlgoED25519, algos)
	}
	// The defaults stay available so a real key change is still detected.
	if !slices.Contains(algos, ssh.KeyAlgoECDSA256) {
		t.Errorf("expected defaults to be kept after the known types, got %v", algos)
	}
}

func TestPreferredHostKeyAlgorithms_RSA(t *testing.T) {
	rsaSigner := testSigner(t, func() (crypto.Signer, error) {
		return rsa.GenerateKey(rand.Reader, 2048)
	})
	known := knownHostsWith(t, "example.com:22", rsaSigner.PublicKey())

	algos := preferredHostKeyAlgorithms(known, "example.com:22")
	want := []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256}
	if len(algos) < 2 || !slices.Equal(algos[:2], want) {
		t.Errorf("expected %v first for an RSA key, got %v", want, algos)
	}
	if slices.Contains(algos, ssh.KeyAlgoRSA) {
		t.Errorf("SHA-1 ssh-rsa must not be offered, got %v", algos)
	}
}

func TestPreferredHostKeyAlgorithms_NonStandardPort(t *testing.T) {
	known := knownHostsWith(t, "gate.example.com:3022", ed25519Signer(t).PublicKey())

	if algos := preferredHostKeyAlgorithms(known, "gate.example.com:3022"); len(algos) == 0 || algos[0] != ssh.KeyAlgoED25519 {
		t.Errorf("expected %s first on port 3022, got %v", ssh.KeyAlgoED25519, algos)
	}
	if algos := preferredHostKeyAlgorithms(known, "gate.example.com:22"); algos != nil {
		t.Errorf("an entry for port 3022 must not match port 22, got %v", algos)
	}
}

// startTestSSHServer serves SSH handshakes on 127.0.0.1 with the given host
// keys and no client authentication, and returns its address.
func startTestSSHServer(t *testing.T, hostKeys ...ssh.Signer) string {
	t.Helper()
	config := &ssh.ServerConfig{NoClientAuth: true}
	for _, key := range hostKeys {
		config.AddHostKey(key)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, chans, reqs, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for newChan := range chans {
					_ = newChan.Reject(ssh.Prohibited, "test server")
				}
			}()
		}
	}()

	return listener.Addr().String()
}

// A server first reached with OpenSSH only has its Ed25519 key recorded, while
// it also serves an ECDSA key. The connection must succeed, not report a
// changed key.
func TestResolveHostKey_OnlyEd25519Known(t *testing.T) {
	edKey := ed25519Signer(t)
	addr := startTestSSHServer(t, ecdsaSigner(t), edKey)

	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	line := knownhosts.Line([]string{addr}, edKey.PublicKey())
	if err := os.WriteFile(filepath.Join(home, ".ssh", "known_hosts"), []byte(line+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	callback, algos, err := ResolveHostKey(nil, addr)
	if err != nil {
		t.Fatalf("ResolveHostKey() error = %v", err)
	}

	// Without the ordering, Go negotiates ECDSA and the key looks changed.
	_, err = ssh.Dial("tcp", addr, &ssh.ClientConfig{User: "test", HostKeyCallback: callback})
	var changedErr *HostKeyChangedError
	if !errors.As(err, &changedErr) {
		t.Fatalf("expected the default negotiation to hit HostKeyChangedError, got %v", err)
	}

	client, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{
		User:              "test",
		HostKeyCallback:   callback,
		HostKeyAlgorithms: algos,
	})
	if err != nil {
		t.Fatalf("connection with the known Ed25519 key refused: %v", err)
	}
	client.Close()
}

// A server whose recorded key is gone must still be reported as changed.
func TestResolveHostKey_RealKeyChangeStillDetected(t *testing.T) {
	addr := startTestSSHServer(t, ecdsaSigner(t))

	line := knownhosts.Line([]string{addr}, ed25519Signer(t).PublicKey())
	t.Setenv("FRANKENDEPLOY_KNOWN_HOSTS", line+"\n")

	callback, algos, err := ResolveHostKey(nil, addr)
	if err != nil {
		t.Fatalf("ResolveHostKey() error = %v", err)
	}

	_, err = ssh.Dial("tcp", addr, &ssh.ClientConfig{
		User:              "test",
		HostKeyCallback:   callback,
		HostKeyAlgorithms: algos,
	})
	var keyErr *knownhosts.KeyError
	if !errors.As(err, &keyErr) || len(keyErr.Want) == 0 {
		t.Fatalf("expected a known_hosts key mismatch, got %v", err)
	}
}
