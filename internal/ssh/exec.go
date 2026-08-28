package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
)

// ExecResult holds the result of a command execution
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// CommandError wraps a non-zero exit code with stderr context.
type CommandError struct {
	ExitCode int
	Stderr   string
}

func (e *CommandError) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		return fmt.Sprintf("command failed (exit %d)", e.ExitCode)
	}
	return fmt.Sprintf("command failed (exit %d): %s", e.ExitCode, msg)
}

// Err returns a CommandError if ExitCode is non-zero, nil otherwise.
func (r *ExecResult) Err() error {
	if r.ExitCode != 0 {
		return &CommandError{ExitCode: r.ExitCode, Stderr: r.Stderr}
	}
	return nil
}

// Exec executes a command on the remote server
func (c *Client) Exec(ctx context.Context, command string) (*ExecResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	session, err := c.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = runSessionWithContext(ctx, session, command)

	result := &ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}

	if err != nil {
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitStatus()
		} else {
			return result, fmt.Errorf("failed to execute command: %w", err)
		}
	}

	return result, nil
}

// runSessionWithContext runs a session command while honoring context
// cancellation: a remote command that hangs (interactive apt prompt, stuck
// docker build) no longer blocks the CLI past Ctrl+C or a timeout — the
// session is closed and the context error returned.
func runSessionWithContext(ctx context.Context, session *ssh.Session, command string) error {
	done := make(chan error, 1)
	go func() {
		done <- session.Run(command)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		// Closing the session tears down the remote command's channel;
		// the goroutine then finishes on its own.
		_ = session.Close()
		return ctx.Err()
	}
}

// ExecStream executes a command and streams output to stdout/stderr
func (c *Client) ExecStream(ctx context.Context, command string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	session, err := c.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	return runSessionWithContext(ctx, session, command)
}

// GetServerArchitecture returns the server's CPU architecture (e.g., "x86_64", "aarch64")
func (c *Client) GetServerArchitecture(ctx context.Context) (string, error) {
	result, err := c.Exec(ctx, "uname -m")
	if err != nil {
		return "", err
	}

	output := strings.TrimSpace(result.Stdout)
	if err := result.Err(); err != nil {
		return output, err
	}

	return output, nil
}
