package ssh

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
)

// PassphraseReader prompts for the passphrase of an encrypted private key.
type PassphraseReader func(keyPath string) ([]byte, error)

// DefaultPassphraseReader reads the passphrase from the terminal without
// echoing it. It fails when stdin is not a terminal (CI/CD): in that case
// use ssh-agent or FRANKENDEPLOY_SSH_KEY instead.
func DefaultPassphraseReader(keyPath string) ([]byte, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return nil, fmt.Errorf("key %s is passphrase-protected and no terminal is available to prompt "+
			"(use ssh-agent or set FRANKENDEPLOY_SSH_KEY for CI/CD)", keyPath)
	}

	fmt.Printf("Enter passphrase for %s: ", keyPath)
	passphrase, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return nil, fmt.Errorf("failed to read passphrase: %w", err)
	}
	return passphrase, nil
}

// agentSignersFunc returns a function producing the ssh-agent's signers,
// or nil when SSH_AUTH_SOCK is unset or the agent is unreachable.
func agentSignersFunc() func() ([]ssh.Signer, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil
	}

	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil
	}
	// The connection stays open for the lifetime of the process: the SSH
	// library may query the agent again on reconnection.
	return agent.NewClient(conn).Signers
}

// parsePrivateKeyWithPrompt parses a private key, prompting for its
// passphrase when the key is encrypted.
func parsePrivateKeyWithPrompt(data []byte, path string, prompt PassphraseReader) (ssh.Signer, error) {
	signer, err := ssh.ParsePrivateKey(data)
	if err == nil {
		return signer, nil
	}

	var missingErr *ssh.PassphraseMissingError
	if !errors.As(err, &missingErr) && !isPassphraseError(err) {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	if prompt == nil {
		prompt = DefaultPassphraseReader
	}
	passphrase, err := prompt(path)
	if err != nil {
		return nil, err
	}

	signer, err = ssh.ParsePrivateKeyWithPassphrase(data, passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt key %s (wrong passphrase?): %w", path, err)
	}
	return signer, nil
}

// resolveKeyPath returns the private key path to use: the configured path
// (with ~ expanded) or the first default key found in ~/.ssh.
func (c *Client) resolveKeyPath() (string, error) {
	keyPath := c.KeyPath
	if keyPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		for _, p := range []string{
			filepath.Join(homeDir, ".ssh", "id_ed25519"),
			filepath.Join(homeDir, ".ssh", "id_rsa"),
		} {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
		return "", fmt.Errorf("no SSH key found")
	}

	if len(keyPath) >= 2 && keyPath[:2] == "~/" {
		homeDir, _ := os.UserHomeDir()
		keyPath = filepath.Join(homeDir, keyPath[2:])
	}
	return keyPath, nil
}

// authMethods builds the authentication methods:
//  1. FRANKENDEPLOY_SSH_KEY (CI/CD): the provided key only
//  2. a single publickey method combining ssh-agent signers and the key file
//
// Agent and key file MUST share one publickey method: the x/crypto client
// tries at most one AuthMethod per method name, so a second publickey entry
// would silently never be attempted.
func (c *Client) authMethods() ([]ssh.AuthMethod, error) {
	if envKey := os.Getenv("FRANKENDEPLOY_SSH_KEY"); envKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(envKey))
		if err != nil {
			return nil, fmt.Errorf("failed to parse FRANKENDEPLOY_SSH_KEY: %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	}

	agentFn := agentSignersFunc()
	keyPath, err := c.resolveKeyPath()
	if err != nil {
		if agentFn == nil {
			return nil, fmt.Errorf("%w and no ssh-agent running (set FRANKENDEPLOY_SSH_KEY for CI/CD)", err)
		}
		keyPath = ""
	}

	// The callback runs lazily on each connection attempt; the decrypted
	// key file signer is memoized so the passphrase is asked at most once.
	return []ssh.AuthMethod{ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
		return c.collectSigners(agentFn, keyPath)
	})}, nil
}

// collectSigners gathers the agent signers followed by the key file signer.
// The key file passphrase is only prompted when the agent does not already
// hold that key; when the prompt fails but the agent offers other keys,
// authentication proceeds with those instead of aborting.
func (c *Client) collectSigners(agentFn func() ([]ssh.Signer, error), keyPath string) ([]ssh.Signer, error) {
	var signers []ssh.Signer
	if agentFn != nil {
		if agentSigners, err := agentFn(); err == nil {
			signers = append(signers, agentSigners...)
		}
	}

	if keyPath != "" {
		fileSigner, err := c.keyFileSigner(keyPath, signers)
		if err != nil {
			if len(signers) == 0 {
				return nil, err
			}
		} else if fileSigner != nil {
			signers = append(signers, fileSigner)
		}
	}

	if len(signers) == 0 {
		return nil, fmt.Errorf("no usable SSH credentials for %s", keyPath)
	}
	return signers, nil
}

// keyFileSigner loads the key file signer, memoized on the client so a
// passphrase is prompted at most once per process. Returns (nil, nil) when
// the key is encrypted but already present in the agent: the agent signer
// covers it without prompting.
func (c *Client) keyFileSigner(keyPath string, agentSigners []ssh.Signer) (ssh.Signer, error) {
	if c.cachedSigner != nil {
		return c.cachedSigner, nil
	}

	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		var missingErr *ssh.PassphraseMissingError
		if !errors.As(err, &missingErr) && !isPassphraseError(err) {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		if agentHoldsKey(keyPath, agentSigners) {
			return nil, nil
		}
		signer, err = parsePrivateKeyWithPrompt(data, keyPath, c.opts.passphrasePrompt)
		if err != nil {
			return nil, err
		}
	}

	c.cachedSigner = signer
	return signer, nil
}

// agentHoldsKey reports whether the agent already offers the public key
// matching the given private key file (compared via its .pub sibling).
func agentHoldsKey(keyPath string, agentSigners []ssh.Signer) bool {
	pubData, err := os.ReadFile(keyPath + ".pub")
	if err != nil {
		return false
	}
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey(pubData)
	if err != nil {
		return false
	}
	marshaled := pubKey.Marshal()
	for _, s := range agentSigners {
		if bytes.Equal(s.PublicKey().Marshal(), marshaled) {
			return true
		}
	}
	return false
}
