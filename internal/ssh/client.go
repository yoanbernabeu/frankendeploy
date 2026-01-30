package ssh

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Client represents an SSH client connection
type Client struct {
	Host    string
	User    string
	Port    int
	KeyPath string
	client  *ssh.Client
}

// NewClient creates a new SSH client
func NewClient(host, user string, port int, keyPath string) *Client {
	if port == 0 {
		port = 22
	}
	return &Client{
		Host:    host,
		User:    user,
		Port:    port,
		KeyPath: keyPath,
	}
}

// Connect establishes an SSH connection
func (c *Client) Connect() error {
	signer, err := c.loadPrivateKey()
	if err != nil {
		return fmt.Errorf("failed to load private key: %w", err)
	}

	hostKeyCallback, err := c.hostKeyCallback()
	if err != nil {
		return fmt.Errorf("host key verification failed: %w", err)
	}

	config := &ssh.ClientConfig{
		User: c.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         30 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", c.Host, c.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	c.client = client
	return nil
}

// Close closes the SSH connection
func (c *Client) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// IsConnected returns true if the client is connected
func (c *Client) IsConnected() bool {
	return c.client != nil
}

// loadPrivateKey loads the SSH private key
func (c *Client) loadPrivateKey() (ssh.Signer, error) {
	// CI/CD: Check for SSH key in environment variable first
	if envKey := os.Getenv("FRANKENDEPLOY_SSH_KEY"); envKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(envKey))
		if err != nil {
			return nil, fmt.Errorf("failed to parse FRANKENDEPLOY_SSH_KEY: %w", err)
		}
		return signer, nil
	}

	keyPath := c.KeyPath
	if keyPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		// Try common key locations
		keyPaths := []string{
			filepath.Join(homeDir, ".ssh", "id_ed25519"),
			filepath.Join(homeDir, ".ssh", "id_rsa"),
		}
		for _, p := range keyPaths {
			if _, err := os.Stat(p); err == nil {
				keyPath = p
				break
			}
		}
		if keyPath == "" {
			return nil, fmt.Errorf("no SSH key found (set FRANKENDEPLOY_SSH_KEY for CI/CD)")
		}
	}

	// Expand ~ in path
	if len(keyPath) >= 2 && keyPath[:2] == "~/" {
		homeDir, _ := os.UserHomeDir()
		keyPath = filepath.Join(homeDir, keyPath[2:])
	}

	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return signer, nil
}

// hostKeyCallback returns the host key callback function
// SECURITY: This function requires a valid known_hosts file by default
// In CI/CD, set FRANKENDEPLOY_KNOWN_HOSTS with the content of known_hosts
// or FRANKENDEPLOY_SKIP_HOST_KEY_CHECK=true to skip verification (not recommended)
func (c *Client) hostKeyCallback() (ssh.HostKeyCallback, error) {
	// CI/CD: Check for known_hosts content in environment variable
	if knownHostsContent := os.Getenv("FRANKENDEPLOY_KNOWN_HOSTS"); knownHostsContent != "" {
		// Write to temp file for knownhosts.New()
		tmpFile, err := os.CreateTemp("", "known_hosts")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp known_hosts: %w", err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.WriteString(knownHostsContent); err != nil {
			return nil, fmt.Errorf("failed to write temp known_hosts: %w", err)
		}
		tmpFile.Close()

		callback, err := knownhosts.New(tmpFile.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to parse FRANKENDEPLOY_KNOWN_HOSTS: %w", err)
		}
		return callback, nil
	}

	// CI/CD: Option to skip host key verification (use with caution)
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
			"For CI/CD, set FRANKENDEPLOY_KNOWN_HOSTS or FRANKENDEPLOY_SKIP_HOST_KEY_CHECK=true",
			knownHostsPath, c.User, c.Host, c.Port)
	}

	callback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read known_hosts: %w", err)
	}

	return callback, nil
}

// GetClient returns the underlying SSH client
func (c *Client) GetClient() *ssh.Client {
	return c.client
}

// NewSession creates a new SSH session
func (c *Client) NewSession() (*ssh.Session, error) {
	if c.client == nil {
		return nil, fmt.Errorf("not connected")
	}
	return c.client.NewSession()
}
