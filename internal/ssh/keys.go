package ssh

import (
	"bytes"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// SSHKeyInfo contains information about an SSH key
type SSHKeyInfo struct {
	Path        string // Full path to the key file
	Name        string // Key filename (e.g., "id_ed25519")
	Type        string // Key type (e.g., "ed25519", "rsa", "ecdsa")
	IsEncrypted bool   // True if key is passphrase-protected
}

// DiscoverSSHKeys scans ~/.ssh/ for private keys
// Returns keys sorted by preference: ed25519 first, then rsa, then others
func DiscoverSSHKeys() ([]SSHKeyInfo, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}

	sshDir := filepath.Join(homeDir, ".ssh")
	entries, err := os.ReadDir(sshDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read .ssh directory: %w", err)
	}

	var keys []SSHKeyInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Skip public keys and known_hosts
		if strings.HasSuffix(name, ".pub") ||
			name == "known_hosts" ||
			name == "authorized_keys" ||
			name == "config" {
			continue
		}

		// Look for id_* patterns or *.pem files
		if !strings.HasPrefix(name, "id_") && !strings.HasSuffix(name, ".pem") {
			continue
		}

		keyPath := filepath.Join(sshDir, name)
		keyInfo, err := ValidateSSHKey(keyPath)
		if err != nil {
			// Skip invalid key files
			continue
		}

		keys = append(keys, *keyInfo)
	}

	// Sort by preference: ed25519 > rsa > ecdsa > others
	sort.Slice(keys, func(i, j int) bool {
		return keyTypePriority(keys[i].Type) < keyTypePriority(keys[j].Type)
	})

	return keys, nil
}

// keyTypePriority returns sort priority for key types (lower is better)
func keyTypePriority(keyType string) int {
	switch keyType {
	case "ed25519":
		return 1
	case "rsa":
		return 2
	case "ecdsa":
		return 3
	default:
		return 4
	}
}

// ValidateSSHKey validates a key file and returns its info
func ValidateSSHKey(path string) (*SSHKeyInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	keyInfo := &SSHKeyInfo{
		Path: path,
		Name: filepath.Base(path),
	}

	// Try to parse the key to validate it
	_, err = ssh.ParsePrivateKey(data)
	if err != nil {
		// Check if it's a passphrase-protected key
		if isPassphraseError(err) {
			keyInfo.IsEncrypted = true
			keyInfo.Type = detectKeyType(data)
			return keyInfo, nil
		}
		return nil, fmt.Errorf("invalid SSH key: %w", err)
	}

	keyInfo.Type = detectKeyType(data)
	return keyInfo, nil
}

// isPassphraseError checks if the error indicates a passphrase-protected key
func isPassphraseError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "passphrase") ||
		strings.Contains(errStr, "encrypted") ||
		strings.Contains(errStr, "ENCRYPTED")
}

// detectKeyType attempts to detect the key type from the key data.
// For legacy PEM types the header is authoritative. For modern OpenSSH keys,
// the PEM header is always "OPENSSH PRIVATE KEY" regardless of the underlying
// algorithm, so we PEM-decode and inspect the binary blob for the key-type
// marker (e.g. "ssh-ed25519", "ssh-rsa", "ecdsa-sha2-*", "ssh-dss").
func detectKeyType(data []byte) string {
	content := string(data)

	switch {
	case strings.Contains(content, "-----BEGIN RSA PRIVATE KEY-----"):
		return "rsa"
	case strings.Contains(content, "-----BEGIN EC PRIVATE KEY-----"):
		return "ecdsa"
	case strings.Contains(content, "-----BEGIN DSA PRIVATE KEY-----"):
		return "dsa"
	case strings.Contains(content, "-----BEGIN OPENSSH PRIVATE KEY-----"):
		if block, _ := pem.Decode(data); block != nil {
			switch {
			case bytes.Contains(block.Bytes, []byte("ssh-ed25519")),
				bytes.Contains(block.Bytes, []byte("sk-ssh-ed25519")):
				return "ed25519"
			case bytes.Contains(block.Bytes, []byte("ssh-rsa")):
				return "rsa"
			case bytes.Contains(block.Bytes, []byte("ecdsa-sha2")),
				bytes.Contains(block.Bytes, []byte("sk-ecdsa-sha2")):
				return "ecdsa"
			case bytes.Contains(block.Bytes, []byte("ssh-dss")):
				return "dsa"
			}
		}
	}

	return "unknown"
}

// TryConnect attempts to connect to a server with a specific key
// Returns nil on success, error on failure
func TryConnect(host, user string, port int, keyPath string) error {
	if port == 0 {
		port = 22
	}

	// Load the private key
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("failed to read key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return fmt.Errorf("failed to parse key: %w", err)
	}

	// Get host key callback
	hostKeyCallback, err := getHostKeyCallback(host, user, port)
	if err != nil {
		return err
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	client.Close()

	return nil
}

// getHostKeyCallback returns the host key callback for connection testing
func getHostKeyCallback(host, user string, port int) (ssh.HostKeyCallback, error) {
	// Check for CI/CD environment variables first
	if os.Getenv("FRANKENDEPLOY_SKIP_HOST_KEY_CHECK") == "true" {
		return ssh.InsecureIgnoreHostKey(), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}

	knownHostsPath := filepath.Join(homeDir, ".ssh", "known_hosts")

	if _, err := os.Stat(knownHostsPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("SSH known_hosts file not found at %s. "+
			"Please connect to the server manually first with: ssh %s@%s -p %d\n"+
			"For CI/CD, set FRANKENDEPLOY_SKIP_HOST_KEY_CHECK=true",
			knownHostsPath, user, host, port)
	}

	callback, err := newKnownHostsCallback(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read known_hosts: %w", err)
	}

	return callback, nil
}

// newKnownHostsCallback creates a host key callback from a known_hosts file
func newKnownHostsCallback(path string) (ssh.HostKeyCallback, error) {
	return knownhosts.New(path)
}
