package ssh

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverSSHKeys(t *testing.T) {
	// Create a temporary .ssh directory
	tmpDir := t.TempDir()
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh dir: %v", err)
	}

	// Create test key files
	testKeys := map[string]string{
		"id_ed25519": `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACCBHxJnPHwqFPxfF5XHV4SRS15iU7t9bZCdnf4yZgQ/RgAAAJii+kgiovpI
IgAAAAtzc2gtZWQyNTUxOQAAACCBHxJnPHwqFPxfF5XHV4SRS15iU7t9bZCdnf4yZgQ/Rg
AAAEBtVLTqTDQaJxy8YvTKV+0Zcq+6uStMebNlIzLXyuHxboEfEmc8fCoU/F8XlcdXhJFL
XmJTu31tkJ2d/jJmBD9GAAAAEHRlc3RAZXhhbXBsZS5jb20BAgMEBQ==
-----END OPENSSH PRIVATE KEY-----`,
		"id_rsa": `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA0Z3VS5JJcds3xfn/ygWyF8PbnGy0AHB7Ls1eFAYFrnkHjC1l
FTJgGpSKvN4bQgvSP8IRJpDHBWO2gQuU/WHtGRBtEkyEZLphMYSW1FfRNs0CWXkh
E5dySk4h8F/tTHyKg8v5aKRNFoC1pNjJ3hRtawH5P8eKOoY1rO5dSAGZL9EHeXR0
qJH1p3a8sYHLceXMfFE7C3BXJRc1JvQaNQ0XaNh9HBjh7RNTKy4VQpLBP+YrqCka
A3N4lPdC1VfhTdqA8gKqN8k8r4N6PfMVqWl1LLRQM1QFjwmJWlIFLgMa6e5h6d/L
jFYDr0eNkK0p5V4sMQh2IViDQRc4VJiNBQ0uNwIDAQABAoIBADm8T4U/5Af6bNis
B3aArDmKl6xEsoW4B3MoPEGFqP1CE0EVNqMhC6Ae5xN/GNDR4wfnNq+EHkFVNT6E
vz/0L3V8RN1l0+GFdH6S7hQk4b1N+KLa9UXV2p1UQ8V5L1ByJH+e0V4sIiZS7CeB
SLj1VYZZP+PfKKKqETfGNwmqRYLDw3Gzq6b5ioHJ7Plpmx9k3pTQo9p+y1Dpq/kq
S8xjP5rN4h7zblU0GpJpkXXq1APGW2zJKj3QfYVdF8c1LI3p8P2oZPCLbJLbNsYk
B1F7j7F9YxDP3QF5M4F+D9yK+OB7MFpQN8c7CUKmU1l7hBnUFHNB1FGPMC0FN9Kw
pkgwVqECgYEA7jU+P3e7BDg1UO+Q3rvX4LS3Af0h4zLJ3G3cJ+m8lPnPfP2b0p8t
8PVaRrR6QJDV5peEfVfT+wFPT8b3p6qzE/F1EJbH7LhXZ6dF8pdM3bLO/7VJfSbm
QQkq3V/eJpDI9FD1K7c1XEVP+b3kvFn5E+5pFAMeQF7L8sypHLH9B+cCgYEA4WD+
c0fEQ6G3fLkP3nFPkr7dqX/A7FqV7nRmP4Fb3P4T1d7VJhJ3rBNMPZ0jGvFp6hIu
qZV0TBF2l5+yk/Rn2e8V8fP7VfUC/K0lncwqBcD7YJnj2QE0nRJ0MbP+zLCBdq9l
Z7DDBJwF7Lg9NG8D7b7+Y5UPkQEd0p8g8M0mQUECgYEA5LY8L9bUVf2U8YBFB7gG
NwUc3Z3jz0Kqo3i5GvNvpfVZvHw7hfNdJMZvj2JBRB3hWQ1QE+LkPcEPH0xqb0vH
1y9bpQLP+0DvLK7NU/F3T8VPbGjS0D0xA3T8Ks1LfMN2EQ0s9g7FWjQ7KpQKQEQ7
ePfVBNnJ3dF6d8L7sE/4y7ECgYAl8P0xQ2S8GlJ1DsB0W7yG5aB5L5C3mVn3D8B+
kfDK0p8qqDZ/h4X8yF3FljD8JQdF2m0hLmJk1NpQp3D3FvH7nE0DnJ3dGK6dW2h4
k4F2F5k8L2VqAHZG6v4r7hYWQCK1H4D8pL8f5F0c3H4B7vH8L7sFQE7dH1pQ2s8G
lJ1DsQKBgCmr0p8q4D8F2m0hLmJk1NpQp3D3FvH7nE0DnJ3dGK6dW2h4k4F2F5k8
L2VqAHZG6v4r7hYWQCK1H4D8pL8f5F0c3H4B7vH8L7sFQE7dH1pQ2s8GlJ1DsB0W
7yG5aB5L5C3mVn3D8B+kfDK0p8qqDZ/h4X8yF3FljD8JQdF2m0h
-----END RSA PRIVATE KEY-----`,
	}

	for name, content := range testKeys {
		path := filepath.Join(sshDir, name)
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatalf("Failed to create test key %s: %v", name, err)
		}
	}

	// Also create a .pub file that should be ignored
	pubPath := filepath.Join(sshDir, "id_ed25519.pub")
	if err := os.WriteFile(pubPath, []byte("ssh-ed25519 AAAA... test@example.com"), 0644); err != nil {
		t.Fatalf("Failed to create .pub file: %v", err)
	}

	// This test can't easily override os.UserHomeDir(), so we test the helper functions instead
	t.Run("detectKeyType", func(t *testing.T) {
		tests := []struct {
			name     string
			content  string
			expected string
		}{
			{
				name:     "ed25519 key",
				content:  testKeys["id_ed25519"],
				expected: "ed25519",
			},
			{
				name:     "rsa key",
				content:  testKeys["id_rsa"],
				expected: "rsa",
			},
			{
				name:     "unknown key",
				content:  "-----BEGIN UNKNOWN PRIVATE KEY-----",
				expected: "unknown",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := detectKeyType([]byte(tt.content))
				if result != tt.expected {
					t.Errorf("detectKeyType() = %v, want %v", result, tt.expected)
				}
			})
		}
	})
}

func TestValidateSSHKey(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a valid ed25519 key
	validKey := `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACCBHxJnPHwqFPxfF5XHV4SRS15iU7t9bZCdnf4yZgQ/RgAAAJii+kgiovpI
IgAAAAtzc2gtZWQyNTUxOQAAACCBHxJnPHwqFPxfF5XHV4SRS15iU7t9bZCdnf4yZgQ/Rg
AAAEBtVLTqTDQaJxy8YvTKV+0Zcq+6uStMebNlIzLXyuHxboEfEmc8fCoU/F8XlcdXhJFL
XmJTu31tkJ2d/jJmBD9GAAAAEHRlc3RAZXhhbXBsZS5jb20BAgMEBQ==
-----END OPENSSH PRIVATE KEY-----`

	validKeyPath := filepath.Join(tmpDir, "id_ed25519")
	if err := os.WriteFile(validKeyPath, []byte(validKey), 0600); err != nil {
		t.Fatalf("Failed to write test key: %v", err)
	}

	t.Run("valid key", func(t *testing.T) {
		info, err := ValidateSSHKey(validKeyPath)
		if err != nil {
			t.Errorf("ValidateSSHKey() error = %v, want nil", err)
			return
		}
		if info.Name != "id_ed25519" {
			t.Errorf("Name = %v, want id_ed25519", info.Name)
		}
		if info.IsEncrypted {
			t.Errorf("IsEncrypted = true, want false")
		}
	})

	t.Run("nonexistent key", func(t *testing.T) {
		_, err := ValidateSSHKey(filepath.Join(tmpDir, "nonexistent"))
		if err == nil {
			t.Error("ValidateSSHKey() error = nil, want error")
		}
	})

	t.Run("invalid key content", func(t *testing.T) {
		invalidPath := filepath.Join(tmpDir, "invalid_key")
		if err := os.WriteFile(invalidPath, []byte("not a key"), 0600); err != nil {
			t.Fatalf("Failed to write invalid key: %v", err)
		}
		_, err := ValidateSSHKey(invalidPath)
		if err == nil {
			t.Error("ValidateSSHKey() error = nil, want error")
		}
	})
}

func TestIsPassphraseError(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected bool
	}{
		{
			name:     "passphrase error",
			errMsg:   "this key is passphrase protected",
			expected: true,
		},
		{
			name:     "encrypted error",
			errMsg:   "key is encrypted",
			expected: true,
		},
		{
			name:     "ENCRYPTED uppercase",
			errMsg:   "ENCRYPTED PRIVATE KEY",
			expected: true,
		},
		{
			name:     "other error",
			errMsg:   "invalid key format",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &testError{msg: tt.errMsg}
			result := isPassphraseError(err)
			if result != tt.expected {
				t.Errorf("isPassphraseError() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestKeyTypePriority(t *testing.T) {
	tests := []struct {
		keyType  string
		expected int
	}{
		{"ed25519", 1},
		{"rsa", 2},
		{"ecdsa", 3},
		{"dsa", 4},
		{"unknown", 4},
	}

	for _, tt := range tests {
		t.Run(tt.keyType, func(t *testing.T) {
			result := keyTypePriority(tt.keyType)
			if result != tt.expected {
				t.Errorf("keyTypePriority(%s) = %v, want %v", tt.keyType, result, tt.expected)
			}
		})
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
