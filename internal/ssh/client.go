package ssh

import (
	"errors"
	"fmt"
	"math"
	"strings"
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
	timeout          time.Duration
	maxRetries       int
	initialDelay     time.Duration
	maxDelay         time.Duration
	passphrasePrompt PassphraseReader
	hostKeyPrompt    HostKeyPrompt
}

func defaultOptions() clientOptions {
	return clientOptions{
		timeout:          DefaultTimeout,
		maxRetries:       DefaultMaxRetries,
		initialDelay:     DefaultInitialDelay,
		maxDelay:         DefaultMaxDelay,
		passphrasePrompt: DefaultPassphraseReader,
		hostKeyPrompt:    DefaultHostKeyPrompt,
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

// WithPassphraseReader sets the prompt used for encrypted key passphrases.
func WithPassphraseReader(r PassphraseReader) ClientOption {
	return func(o *clientOptions) {
		o.passphrasePrompt = r
	}
}

// WithHostKeyPrompt sets the prompt used to confirm unknown host keys (TOFU).
func WithHostKeyPrompt(p HostKeyPrompt) ClientOption {
	return func(o *clientOptions) {
		o.hostKeyPrompt = p
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
	// cachedSigner memoizes the (possibly passphrase-decrypted) key file
	// signer so the passphrase is prompted at most once per process
	cachedSigner ssh.Signer
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
	auths, err := c.authMethods()
	if err != nil {
		return fmt.Errorf("failed to load SSH credentials: %w", err)
	}

	hostKeyCallback, err := ResolveHostKeyCallback(c.opts.hostKeyPrompt)
	if err != nil {
		return fmt.Errorf("host key verification failed: %w", err)
	}

	c.sshConfig = &ssh.ClientConfig{
		User:            c.User,
		Auth:            auths,
		HostKeyCallback: hostKeyCallback,
		Timeout:         c.opts.timeout,
	}

	return c.connectWithRetry()
}

// connectWithRetry attempts to connect with exponential backoff. Only
// network-level errors are retried: authentication and host key failures
// are permanent and surfaced immediately.
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
		if !isRetryableConnError(err) {
			return classifyConnError(addr, err)
		}
		lastErr = err
	}

	return fmt.Errorf("failed to connect to %s after %d attempts: %w", addr, c.opts.maxRetries, lastErr)
}

// isRetryableConnError reports whether a connection error is worth retrying.
// Host key and authentication failures are permanent; retrying them only
// hides the real message behind "failed after N attempts".
func isRetryableConnError(err error) bool {
	var changedErr *HostKeyChangedError
	var unknownErr *HostKeyUnknownError
	var keyErr *knownhosts.KeyError
	if errors.As(err, &changedErr) || errors.As(err, &unknownErr) || errors.As(err, &keyErr) {
		return false
	}

	msg := err.Error()
	if strings.Contains(msg, "unable to authenticate") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "no supported methods remain") {
		return false
	}
	return true
}

// classifyConnError unwraps host key errors so the user sees the dedicated
// message instead of the generic handshake wrapper.
func classifyConnError(addr string, err error) error {
	var changedErr *HostKeyChangedError
	if errors.As(err, &changedErr) {
		return changedErr
	}
	var unknownErr *HostKeyUnknownError
	if errors.As(err, &unknownErr) {
		return unknownErr
	}
	return fmt.Errorf("failed to connect to %s: %w", addr, err)
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
