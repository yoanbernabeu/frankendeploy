package ssh

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// SSH client defaults
const (
	DefaultTimeout      = 30 * time.Second
	DefaultMaxRetries   = 3
	DefaultInitialDelay = 1 * time.Second
	DefaultMaxDelay     = 10 * time.Second
)

// ClientOption is a functional option for configuring the SSH client.
type ClientOption func(*clientOptions)

type clientOptions struct {
	timeout      time.Duration
	maxRetries   int
	initialDelay time.Duration
	maxDelay     time.Duration
}

func defaultOptions() clientOptions {
	return clientOptions{
		timeout:      DefaultTimeout,
		maxRetries:   DefaultMaxRetries,
		initialDelay: DefaultInitialDelay,
		maxDelay:     DefaultMaxDelay,
	}
}

// WithTimeout sets the SSH connection timeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.timeout = d
	}
}

// WithRetries sets the maximum number of connection retries.
func WithRetries(n int) ClientOption {
	return func(o *clientOptions) {
		o.maxRetries = n
	}
}

// WithInitialDelay sets the initial delay between retries.
func WithInitialDelay(d time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.initialDelay = d
	}
}

// WithMaxDelay sets the maximum delay between retries.
func WithMaxDelay(d time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.maxDelay = d
	}
}

// Client represents an SSH client connection
type Client struct {
	Host    string
	User    string
	Port    int
	KeyPath string
	client  *ssh.Client
	opts    clientOptions
	// sshConfig is stored to allow reconnection without reloading keys
	sshConfig *ssh.ClientConfig
}

// NewClient creates a new SSH client.
// Accepts optional ClientOption arguments for configuration (backward compatible).
func NewClient(host, user string, port int, keyPath string, opts ...ClientOption) *Client {
	if port == 0 {
		port = 22
	}
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return &Client{
		Host:    host,
		User:    user,
		Port:    port,
		KeyPath: keyPath,
		opts:    o,
	}
}

// Connect establishes an SSH connection with retry and exponential backoff.
func (c *Client) Connect() error {
	signer, err := c.loadPrivateKey()
	if err != nil {
		return fmt.Errorf("failed to load private key: %w", err)
	}

	hostKeyCallback, err := c.hostKeyCallback()
	if err != nil {
		return fmt.Errorf("host key verification failed: %w", err)
	}

	c.sshConfig = &ssh.ClientConfig{
		User: c.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         c.opts.timeout,
	}

	return c.connectWithRetry()
}

// connectWithRetry attempts to connect with exponential backoff.
func (c *Client) connectWithRetry() error {
	addr := fmt.Sprintf("%s:%d", c.Host, c.Port)
	var lastErr error

	for attempt := 0; attempt < c.opts.maxRetries; attempt++ {
		if attempt > 0 {
			delay := c.backoffDelay(attempt)
			time.Sleep(delay)
		}

		client, err := ssh.Dial("tcp", addr, c.sshConfig)
		if err == nil {
			c.client = client
			return nil
		}
		lastErr = err
	}

	return fmt.Errorf("failed to connect to %s after %d attempts: %w", addr, c.opts.maxRetries, lastErr)
}

// backoffDelay returns the delay for the given retry attempt using exponential backoff.
func (c *Client) backoffDelay(attempt int) time.Duration {
	delay := time.Duration(float64(c.opts.initialDelay) * math.Pow(2, float64(attempt-1)))
	if delay > c.opts.maxDelay {
		delay = c.opts.maxDelay
	}
	return delay
}

// Close closes the SSH connection
func (c *Client) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// Reconnect closes the existing connection and establishes a new one.
// Requires that Connect() was called previously (sshConfig is stored).
func (c *Client) Reconnect() error {
	if c.sshConfig == nil {
		return fmt.Errorf("cannot reconnect: no previous connection configuration")
	}
	// Close existing connection if any
	if c.client != nil {
		c.client.Close()
		c.client = nil
	}
	return c.connectWithRetry()
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

// NewSession creates a new SSH session.
// If the session creation fails, it attempts to reconnect once and retry.
func (c *Client) NewSession() (*ssh.Session, error) {
	if c.client == nil {
		return nil, fmt.Errorf("not connected")
	}

	session, err := c.client.NewSession()
	if err == nil {
		return session, nil
	}

	// Attempt auto-reconnect once
	if c.sshConfig != nil {
		if reconnErr := c.Reconnect(); reconnErr != nil {
			return nil, fmt.Errorf("session failed and reconnect failed: %w (original: %v)", reconnErr, err)
		}
		return c.client.NewSession()
	}

	return nil, fmt.Errorf("failed to create session: %w", err)
}
