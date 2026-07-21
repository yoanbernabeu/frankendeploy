package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// generateKeyPEM returns a PEM-encoded ed25519 private key, optionally
// encrypted with the given passphrase.
func generateKeyPEM(t *testing.T, passphrase string) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	var block *pem.Block
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(priv, "test")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(priv, "test", []byte(passphrase))
	}
	if err != nil {
		t.Fatalf("failed to marshal key: %v", err)
	}
	return pem.EncodeToMemory(block)
}

func TestParsePrivateKeyWithPrompt_Unencrypted(t *testing.T) {
	data := generateKeyPEM(t, "")

	prompted := false
	signer, err := parsePrivateKeyWithPrompt(data, "/key", func(keyPath string) ([]byte, error) {
		prompted = true
		return nil, nil
	})
	if err != nil {
		t.Fatalf("parsePrivateKeyWithPrompt() error = %v", err)
	}
	if signer == nil {
		t.Fatal("expected a signer")
	}
	if prompted {
		t.Error("prompt was called for an unencrypted key")
	}
}

func TestParsePrivateKeyWithPrompt_Encrypted(t *testing.T) {
	data := generateKeyPEM(t, "secret")

	var promptedPath string
	signer, err := parsePrivateKeyWithPrompt(data, "/home/user/.ssh/id_ed25519", func(keyPath string) ([]byte, error) {
		promptedPath = keyPath
		return []byte("secret"), nil
	})
	if err != nil {
		t.Fatalf("parsePrivateKeyWithPrompt() error = %v", err)
	}
	if signer == nil {
		t.Fatal("expected a signer")
	}
	if promptedPath != "/home/user/.ssh/id_ed25519" {
		t.Errorf("prompt received path %q, want the key path", promptedPath)
	}
}

func TestParsePrivateKeyWithPrompt_WrongPassphrase(t *testing.T) {
	data := generateKeyPEM(t, "secret")

	_, err := parsePrivateKeyWithPrompt(data, "/key", func(keyPath string) ([]byte, error) {
		return []byte("wrong"), nil
	})
	if err == nil {
		t.Fatal("expected error with wrong passphrase")
	}
}

func TestParsePrivateKeyWithPrompt_PromptError(t *testing.T) {
	data := generateKeyPEM(t, "secret")

	promptErr := errors.New("no terminal available")
	_, err := parsePrivateKeyWithPrompt(data, "/key", func(keyPath string) ([]byte, error) {
		return nil, promptErr
	})
	if !errors.Is(err, promptErr) {
		t.Fatalf("expected prompt error to propagate, got: %v", err)
	}
}

func TestParsePrivateKeyWithPrompt_InvalidKey(t *testing.T) {
	prompted := false
	_, err := parsePrivateKeyWithPrompt([]byte("not a key"), "/key", func(keyPath string) ([]byte, error) {
		prompted = true
		return nil, nil
	})
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
	if prompted {
		t.Error("prompt was called for an invalid (non-encrypted) key")
	}
}

// startFakeAgent starts an in-process ssh-agent on a unix socket and returns
// the socket path. A short temp dir is used directly: t.TempDir() paths embed
// the test name and can exceed the macOS 104-char unix socket path limit.
func startFakeAgent(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "agent")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	sockPath := filepath.Join(dir, "a.sock")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to listen on unix socket: %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	keyring := agent.NewKeyring()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() { _ = agent.ServeAgent(keyring, conn) }()
		}
	}()
	return sockPath
}

func TestAgentSignersFunc_NoSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	if fn := agentSignersFunc(); fn != nil {
		t.Error("expected nil signers func without SSH_AUTH_SOCK")
	}
}

func TestAgentSignersFunc_UnreachableSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(t.TempDir(), "nonexistent.sock"))
	if fn := agentSignersFunc(); fn != nil {
		t.Error("expected nil signers func for unreachable agent socket")
	}
}

func TestAgentSignersFunc_WithAgent(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", startFakeAgent(t))
	fn := agentSignersFunc()
	if fn == nil {
		t.Fatal("expected signers func with a reachable ssh-agent")
	}
	signers, err := fn()
	if err != nil {
		t.Fatalf("signers func error = %v", err)
	}
	if len(signers) != 0 {
		t.Errorf("expected empty keyring, got %d signers", len(signers))
	}
}

func TestClientAuthMethods_EnvKey(t *testing.T) {
	data := generateKeyPEM(t, "")
	t.Setenv("FRANKENDEPLOY_SSH_KEY", string(data))
	t.Setenv("SSH_AUTH_SOCK", "")

	client := NewClient("host", "user", 22, "")
	methods, err := client.authMethods()
	if err != nil {
		t.Fatalf("authMethods() error = %v", err)
	}
	if len(methods) != 1 {
		t.Errorf("expected exactly 1 auth method with FRANKENDEPLOY_SSH_KEY, got %d", len(methods))
	}
}

func TestClientAuthMethods_KeyFileOnly(t *testing.T) {
	t.Setenv("FRANKENDEPLOY_SSH_KEY", "")
	t.Setenv("SSH_AUTH_SOCK", "")

	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyPath, generateKeyPEM(t, ""), 0600); err != nil {
		t.Fatalf("failed to write key: %v", err)
	}

	client := NewClient("host", "user", 22, keyPath)
	methods, err := client.authMethods()
	if err != nil {
		t.Fatalf("authMethods() error = %v", err)
	}
	if len(methods) != 1 {
		t.Errorf("expected 1 auth method (key file), got %d", len(methods))
	}
}

func TestClientAuthMethods_AgentPlusKeyFile(t *testing.T) {
	t.Setenv("FRANKENDEPLOY_SSH_KEY", "")
	t.Setenv("SSH_AUTH_SOCK", startFakeAgent(t))

	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyPath, generateKeyPEM(t, ""), 0600); err != nil {
		t.Fatalf("failed to write key: %v", err)
	}

	client := NewClient("host", "user", 22, keyPath)
	methods, err := client.authMethods()
	if err != nil {
		t.Fatalf("authMethods() error = %v", err)
	}
	// Agent and key file MUST be combined into a single publickey method:
	// the x/crypto client only ever tries one AuthMethod per method name.
	if len(methods) != 1 {
		t.Errorf("expected 1 combined publickey method, got %d", len(methods))
	}
}

func TestClientAuthMethods_AgentOnlyNoKey(t *testing.T) {
	t.Setenv("FRANKENDEPLOY_SSH_KEY", "")
	t.Setenv("SSH_AUTH_SOCK", startFakeAgent(t))

	// Point HOME to an empty dir so no default key is discovered
	t.Setenv("HOME", t.TempDir())

	client := NewClient("host", "user", 22, "")
	methods, err := client.authMethods()
	if err != nil {
		t.Fatalf("authMethods() should succeed with agent only, got error = %v", err)
	}
	if len(methods) != 1 {
		t.Errorf("expected 1 auth method (agent only), got %d", len(methods))
	}
}

func TestClientAuthMethods_NoAgentNoKey(t *testing.T) {
	t.Setenv("FRANKENDEPLOY_SSH_KEY", "")
	t.Setenv("SSH_AUTH_SOCK", "")
	t.Setenv("HOME", t.TempDir())

	client := NewClient("host", "user", 22, "")
	_, err := client.authMethods()
	if err == nil {
		t.Fatal("expected error without agent and without key")
	}
}

// signerFromPEM parses a PEM key into a signer for tests
func signerFromPEM(t *testing.T, data []byte) ssh.Signer {
	t.Helper()
	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		t.Fatalf("failed to parse key: %v", err)
	}
	return signer
}

// writeKeyPair writes a private key and its .pub sibling, returning the
// private key path and the parsed signer
func writeKeyPair(t *testing.T, dir, passphrase string) (string, ssh.Signer) {
	t.Helper()
	plain := generateKeyPEM(t, "")
	signer := signerFromPEM(t, plain)

	data := plain
	if passphrase != "" {
		// Re-encrypt the same key so the .pub matches
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("failed to generate key: %v", err)
		}
		block, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "test", []byte(passphrase))
		if err != nil {
			t.Fatalf("failed to marshal key: %v", err)
		}
		data = pem.EncodeToMemory(block)
		plainBlock, err := ssh.MarshalPrivateKey(priv, "test")
		if err != nil {
			t.Fatalf("failed to marshal key: %v", err)
		}
		signer = signerFromPEM(t, pem.EncodeToMemory(plainBlock))
	}

	keyPath := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(keyPath, data, 0600); err != nil {
		t.Fatalf("failed to write key: %v", err)
	}
	pub := ssh.MarshalAuthorizedKey(signer.PublicKey())
	if err := os.WriteFile(keyPath+".pub", pub, 0644); err != nil {
		t.Fatalf("failed to write pub key: %v", err)
	}
	return keyPath, signer
}

func TestCollectSigners_AgentEmptyPlusPlainKey(t *testing.T) {
	keyPath, _ := writeKeyPair(t, t.TempDir(), "")
	client := NewClient("host", "user", 22, keyPath)

	agentFn := func() ([]ssh.Signer, error) { return nil, nil }
	signers, err := client.collectSigners(agentFn, keyPath)
	if err != nil {
		t.Fatalf("collectSigners() error = %v", err)
	}
	if len(signers) != 1 {
		t.Errorf("expected 1 signer (key file), got %d", len(signers))
	}
}

func TestCollectSigners_AgentKeysPlusPlainKey(t *testing.T) {
	keyPath, _ := writeKeyPair(t, t.TempDir(), "")
	otherSigner := signerFromPEM(t, generateKeyPEM(t, ""))
	client := NewClient("host", "user", 22, keyPath)

	agentFn := func() ([]ssh.Signer, error) { return []ssh.Signer{otherSigner}, nil }
	signers, err := client.collectSigners(agentFn, keyPath)
	if err != nil {
		t.Fatalf("collectSigners() error = %v", err)
	}
	if len(signers) != 2 {
		t.Errorf("expected 2 signers (agent + key file), got %d", len(signers))
	}
	// Agent signers come first
	if signers[0] != otherSigner {
		t.Error("agent signer must come before the key file signer")
	}
}

func TestCollectSigners_EncryptedKeyPrompted(t *testing.T) {
	keyPath, _ := writeKeyPair(t, t.TempDir(), "secret")
	client := NewClient("host", "user", 22, keyPath,
		WithPassphraseReader(func(kp string) ([]byte, error) { return []byte("secret"), nil }))

	signers, err := client.collectSigners(nil, keyPath)
	if err != nil {
		t.Fatalf("collectSigners() error = %v", err)
	}
	if len(signers) != 1 {
		t.Errorf("expected 1 signer (decrypted key file), got %d", len(signers))
	}
}

func TestCollectSigners_EncryptedKeyInAgentNotPrompted(t *testing.T) {
	dir := t.TempDir()
	keyPath, signer := writeKeyPair(t, dir, "secret")

	prompted := false
	client := NewClient("host", "user", 22, keyPath,
		WithPassphraseReader(func(kp string) ([]byte, error) {
			prompted = true
			return nil, errors.New("should not be called")
		}))

	// The agent already holds the key matching keyPath.pub
	agentFn := func() ([]ssh.Signer, error) { return []ssh.Signer{signer}, nil }
	signers, err := client.collectSigners(agentFn, keyPath)
	if err != nil {
		t.Fatalf("collectSigners() error = %v", err)
	}
	if prompted {
		t.Error("passphrase prompted although the agent already holds the key")
	}
	if len(signers) != 1 {
		t.Errorf("expected 1 signer (agent), got %d", len(signers))
	}
}

func TestCollectSigners_PromptErrorFallsBackToAgent(t *testing.T) {
	keyPath, _ := writeKeyPair(t, t.TempDir(), "secret")
	otherSigner := signerFromPEM(t, generateKeyPEM(t, ""))

	client := NewClient("host", "user", 22, keyPath,
		WithPassphraseReader(func(kp string) ([]byte, error) {
			return nil, errors.New("no terminal")
		}))

	// Agent holds an unrelated key: auth should proceed with it
	agentFn := func() ([]ssh.Signer, error) { return []ssh.Signer{otherSigner}, nil }
	signers, err := client.collectSigners(agentFn, keyPath)
	if err != nil {
		t.Fatalf("collectSigners() should fall back to agent signers, got error = %v", err)
	}
	if len(signers) != 1 {
		t.Errorf("expected 1 signer (agent fallback), got %d", len(signers))
	}
}

func TestCollectSigners_PromptErrorNoAgentFails(t *testing.T) {
	keyPath, _ := writeKeyPair(t, t.TempDir(), "secret")

	promptErr := errors.New("no terminal")
	client := NewClient("host", "user", 22, keyPath,
		WithPassphraseReader(func(kp string) ([]byte, error) { return nil, promptErr }))

	_, err := client.collectSigners(nil, keyPath)
	if !errors.Is(err, promptErr) {
		t.Fatalf("expected prompt error, got: %v", err)
	}
}

func TestCollectSigners_PassphrasePromptedOnce(t *testing.T) {
	keyPath, _ := writeKeyPair(t, t.TempDir(), "secret")

	promptCount := 0
	client := NewClient("host", "user", 22, keyPath,
		WithPassphraseReader(func(kp string) ([]byte, error) {
			promptCount++
			return []byte("secret"), nil
		}))

	for i := 0; i < 3; i++ {
		if _, err := client.collectSigners(nil, keyPath); err != nil {
			t.Fatalf("collectSigners() error = %v", err)
		}
	}
	if promptCount != 1 {
		t.Errorf("passphrase prompted %d times, want 1 (memoized)", promptCount)
	}
}

func TestIsRetryableConnError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{
			name:      "network error is retryable",
			err:       errors.New("dial tcp 192.0.2.1:22: connect: connection refused"),
			retryable: true,
		},
		{
			name:      "timeout is retryable",
			err:       errors.New("dial tcp 192.0.2.1:22: i/o timeout"),
			retryable: true,
		},
		{
			name:      "auth failure is not retryable",
			err:       errors.New("ssh: handshake failed: ssh: unable to authenticate, attempted methods [none publickey], no supported methods remain"),
			retryable: false,
		},
		{
			name:      "host key changed is not retryable",
			err:       fmt.Errorf("ssh: handshake failed: %w", &HostKeyChangedError{Host: "example.com", Fingerprint: "SHA256:abc"}),
			retryable: false,
		},
		{
			name:      "host key unknown is not retryable",
			err:       fmt.Errorf("ssh: handshake failed: %w", &HostKeyUnknownError{Host: "example.com", Fingerprint: "SHA256:abc"}),
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryableConnError(tt.err); got != tt.retryable {
				t.Errorf("isRetryableConnError() = %v, want %v", got, tt.retryable)
			}
		})
	}
}
