package ssh

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
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

// ResolveHostKey returns the host key verification callback used by every SSH
// connection (deploy, server add, TryConnect...), and the host key algorithms
// to negotiate with addr ("host:port"). A nil algorithm list means the
// library defaults.
//
// Resolution order:
//  1. FRANKENDEPLOY_KNOWN_HOSTS: known_hosts content for CI/CD (strict, no TOFU)
//  2. FRANKENDEPLOY_SKIP_HOST_KEY_CHECK=true: skip verification (not recommended)
//  3. ~/.ssh/known_hosts with trust-on-first-use via prompt
func ResolveHostKey(prompt HostKeyPrompt, addr string) (ssh.HostKeyCallback, []string, error) {
	if content := os.Getenv("FRANKENDEPLOY_KNOWN_HOSTS"); content != "" {
		tmpFile, err := os.CreateTemp("", "known_hosts")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create temp known_hosts: %w", err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.WriteString(content); err != nil {
			tmpFile.Close()
			return nil, nil, fmt.Errorf("failed to write temp known_hosts: %w", err)
		}
		tmpFile.Close()

		callback, err := knownhosts.New(tmpFile.Name())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse FRANKENDEPLOY_KNOWN_HOSTS: %w", err)
		}
		return callback, preferredHostKeyAlgorithms(callback, addr), nil
	}

	if os.Getenv("FRANKENDEPLOY_SKIP_HOST_KEY_CHECK") == "true" {
		return ssh.InsecureIgnoreHostKey(), nil, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot determine home directory: %w", err)
	}

	path := filepath.Join(homeDir, ".ssh", "known_hosts")
	callback, err := knownHostsCallbackWithTOFU(path, prompt)
	if err != nil {
		return nil, nil, err
	}

	// The TOFU wrapper would prompt for the probe key: query the plain file.
	base, err := knownhosts.New(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read known_hosts: %w", err)
	}
	return callback, preferredHostKeyAlgorithms(base, addr), nil
}

// preferredHostKeyAlgorithms returns the host key algorithms to offer to addr:
// those matching the key types known_hosts already holds for it, then the
// library defaults. It returns nil when the host is not known yet.
//
// Go offers ECDSA before Ed25519, OpenSSH the other way round. A server first
// reached with ssh(1) only has its Ed25519 key in known_hosts; without this
// ordering Go negotiates ECDSA and the lookup reports a changed key, a false
// man-in-the-middle alert. Known types come first but the defaults are kept,
// so a server that really lost its key still gets HostKeyChangedError.
func preferredHostKeyAlgorithms(known ssh.HostKeyCallback, addr string) []string {
	// Probe with a throwaway key: the KeyError lists the recorded keys.
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil
	}
	probe, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil
	}

	// knownhosts checks the hostname first; the remote address only has to
	// parse, and an empty one never matches a known_hosts entry.
	var keyErr *knownhosts.KeyError
	if !errors.As(known(addr, &net.TCPAddr{}, probe), &keyErr) || len(keyErr.Want) == 0 {
		return nil
	}

	defaults := ssh.SupportedAlgorithms().HostKeys
	var preferred []string
	for _, want := range keyErr.Want {
		for _, algo := range algorithmsForKeyType(want.Key.Type()) {
			if slices.Contains(defaults, algo) && !slices.Contains(preferred, algo) {
				preferred = append(preferred, algo)
			}
		}
	}
	if len(preferred) == 0 {
		return nil
	}

	for _, algo := range defaults {
		if !slices.Contains(preferred, algo) {
			preferred = append(preferred, algo)
		}
	}
	return preferred
}

// algorithmsForKeyType maps a known_hosts key type to the signature
// algorithms that prove it. An RSA key is negotiated as rsa-sha2-*.
func algorithmsForKeyType(keyType string) []string {
	if keyType == ssh.KeyAlgoRSA {
		return []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256}
	}
	return []string{keyType}
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
