package ssh

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
)

// HostKeyPrompt asks the user whether to trust an unknown host key
// (trust-on-first-use). It receives the normalized host, the key type
// and the SHA256 fingerprint, and returns true to accept the key.
type HostKeyPrompt func(host, keyType, fingerprint string) bool

// HostKeyChangedError is returned when the server's host key does not match
// the one recorded in known_hosts. This is never retried and never subject
// to TOFU: it either means the server was reinstalled or a MITM attack.
type HostKeyChangedError struct {
	Host        string
	Fingerprint string
}

func (e *HostKeyChangedError) Error() string {
	return fmt.Sprintf("host key for %s has changed (fingerprint: %s)\n"+
		"This happens when the server was reinstalled or recreated, but could also indicate a man-in-the-middle attack.\n"+
		"If you recently recreated this server, remove the old key with:\n"+
		"  ssh-keygen -R %s", e.Host, e.Fingerprint, e.Host)
}

// HostKeyUnknownError is returned when connecting to an unknown host and the
// key was not confirmed (non-interactive session or user refusal).
type HostKeyUnknownError struct {
	Host        string
	Fingerprint string
}

func (e *HostKeyUnknownError) Error() string {
	return fmt.Sprintf("host key for %s (fingerprint: %s) is not known and was not confirmed\n"+
		"Run the command interactively to review and accept the key, "+
		"or set FRANKENDEPLOY_KNOWN_HOSTS with the known_hosts content for CI/CD", e.Host, e.Fingerprint)
}

// DefaultHostKeyPrompt interactively asks the user to confirm an unknown
// host key, mimicking the standard OpenSSH first-connection prompt.
// It refuses automatically when stdin is not a terminal.
func DefaultHostKeyPrompt(host, keyType, fingerprint string) bool {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false
	}

	fmt.Printf("The authenticity of host '%s' can't be established.\n", host)
	fmt.Printf("%s key fingerprint is %s.\n", keyType, fingerprint)
	fmt.Print("Are you sure you want to continue connecting (yes/no)? ")

	// A partial line before EOF (piped input without trailing newline) is
	// still a valid answer, so only bail out when nothing was read.
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "yes" || answer == "y"
}

// ResolveHostKeyCallback returns the host key verification callback used by
// every SSH connection (deploy, server add, TryConnect...).
//
// Resolution order:
//  1. FRANKENDEPLOY_KNOWN_HOSTS: known_hosts content for CI/CD (strict, no TOFU)
//  2. FRANKENDEPLOY_SKIP_HOST_KEY_CHECK=true: skip verification (not recommended)
//  3. ~/.ssh/known_hosts with trust-on-first-use via prompt
func ResolveHostKeyCallback(prompt HostKeyPrompt) (ssh.HostKeyCallback, error) {
	if content := os.Getenv("FRANKENDEPLOY_KNOWN_HOSTS"); content != "" {
		tmpFile, err := os.CreateTemp("", "known_hosts")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp known_hosts: %w", err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.WriteString(content); err != nil {
			tmpFile.Close()
			return nil, fmt.Errorf("failed to write temp known_hosts: %w", err)
		}
		tmpFile.Close()

		callback, err := knownhosts.New(tmpFile.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to parse FRANKENDEPLOY_KNOWN_HOSTS: %w", err)
		}
		return callback, nil
	}

	if os.Getenv("FRANKENDEPLOY_SKIP_HOST_KEY_CHECK") == "true" {
		return ssh.InsecureIgnoreHostKey(), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}

	return knownHostsCallbackWithTOFU(filepath.Join(homeDir, ".ssh", "known_hosts"), prompt)
}

// knownHostsCallbackWithTOFU wraps a knownhosts callback with
// trust-on-first-use: unknown hosts are confirmed via prompt and appended to
// the known_hosts file; key mismatches are surfaced as HostKeyChangedError.
func knownHostsCallbackWithTOFU(path string, prompt HostKeyPrompt) (ssh.HostKeyCallback, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, fmt.Errorf("failed to create %s: %w", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, nil, 0600); err != nil {
			return nil, fmt.Errorf("failed to create %s: %w", path, err)
		}
	}

	base, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read known_hosts: %w", err)
	}

	// Keys accepted during this process: the base callback is parsed once,
	// so reconnections must not prompt again for a key already accepted.
	var mu sync.Mutex
	accepted := make(map[string]bool)

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := base(hostname, remote, key)
		if err == nil {
			return nil
		}

		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			return err
		}

		host := knownhosts.Normalize(hostname)
		fingerprint := ssh.FingerprintSHA256(key)

		if len(keyErr.Want) > 0 {
			return &HostKeyChangedError{Host: host, Fingerprint: fingerprint}
		}

		mu.Lock()
		alreadyAccepted := accepted[host+"|"+fingerprint]
		mu.Unlock()
		if alreadyAccepted {
			return nil
		}

		if prompt == nil || !prompt(host, key.Type(), fingerprint) {
			return &HostKeyUnknownError{Host: host, Fingerprint: fingerprint}
		}

		if err := appendKnownHost(path, hostname, key); err != nil {
			return fmt.Errorf("failed to record host key: %w", err)
		}

		mu.Lock()
		accepted[host+"|"+fingerprint] = true
		mu.Unlock()
		return nil
	}, nil
}

// appendKnownHost appends the host key to the known_hosts file in the
// standard OpenSSH format.
func appendKnownHost(path, hostname string, key ssh.PublicKey) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = fmt.Fprintln(f, knownhosts.Line([]string{hostname}, key))
	return err
}
