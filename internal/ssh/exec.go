package ssh

import (
	"bytes"
	"fmt"
	"io"
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

// Exec executes a command on the remote server
func (c *Client) Exec(command string) (*ExecResult, error) {
	session, err := c.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = session.Run(command)

	result := &ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}

	if err != nil {
		if exitError, ok := err.(*ExitError); ok {
			result.ExitCode = exitError.ExitStatus()
		} else {
			return result, fmt.Errorf("failed to execute command: %w", err)
		}
	}

	return result, nil
}

// ExecStream executes a command and streams output to stdout/stderr
func (c *Client) ExecStream(command string) error {
	session, err := c.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	return session.Run(command)
}

// ExecWithOutput executes a command and returns combined output
func (c *Client) ExecWithOutput(command string) (string, error) {
	result, err := c.Exec(command)
	if err != nil {
		return "", err
	}

	output := strings.TrimSpace(result.Stdout)
	if result.ExitCode != 0 {
		errMsg := strings.TrimSpace(result.Stderr)
		if errMsg == "" {
			errMsg = output
		}
		return output, fmt.Errorf("command failed (exit %d): %s", result.ExitCode, errMsg)
	}

	return output, nil
}

// GetServerArchitecture returns the server's CPU architecture (e.g., "x86_64", "aarch64")
func (c *Client) GetServerArchitecture() (string, error) {
	return c.ExecWithOutput("uname -m")
}

// ExecMultiple executes multiple commands in sequence
func (c *Client) ExecMultiple(commands []string) error {
	for _, cmd := range commands {
		result, err := c.Exec(cmd)
		if err != nil {
			return fmt.Errorf("failed to execute '%s': %w", cmd, err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("command '%s' failed (exit %d): %s",
				cmd, result.ExitCode, result.Stderr)
		}
	}
	return nil
}

// Shell opens an interactive shell
func (c *Client) Shell() error {
	session, err := c.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	// Set up terminal modes
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	// Request pseudo terminal
	if err := session.RequestPty("xterm", 40, 80, modes); err != nil {
		return fmt.Errorf("failed to request pty: %w", err)
	}

	// Set up pipes
	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	// Start shell
	if err := session.Shell(); err != nil {
		return fmt.Errorf("failed to start shell: %w", err)
	}

	return session.Wait()
}

// ExecInDirectory executes a command in a specific directory
func (c *Client) ExecInDirectory(dir, command string) (*ExecResult, error) {
	fullCommand := fmt.Sprintf("cd %s && %s", dir, command)
	return c.Exec(fullCommand)
}

// ExitError represents an SSH command exit error
type ExitError struct {
	exitStatus int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit status %d", e.exitStatus)
}

func (e *ExitError) ExitStatus() int {
	return e.exitStatus
}


// StreamOutput streams command output with a prefix
func (c *Client) StreamOutput(command string, prefix string, out io.Writer) error {
	session, err := c.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := session.Start(command); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Stream output with prefix
	go streamWithPrefix(stdout, out, prefix)
	go streamWithPrefix(stderr, out, prefix)

	return session.Wait()
}

func streamWithPrefix(r io.Reader, w io.Writer, prefix string) {
	buf := make([]byte, 1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			lines := strings.Split(string(buf[:n]), "\n")
			for _, line := range lines {
				if line != "" {
					fmt.Fprintf(w, "%s%s\n", prefix, line)
				}
			}
		}
		if err != nil {
			break
		}
	}
}
