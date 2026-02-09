package ssh

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/yoanbernabeu/frankendeploy/internal/security"
)

// UploadFile uploads a local file to the remote server
func (c *Client) UploadFile(localPath, remotePath string) error {
	// Open local file
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer localFile.Close()

	// Get file info
	fileInfo, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat local file: %w", err)
	}

	// Create session
	session, err := c.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	// Create remote directory if needed
	remoteDir := filepath.Dir(remotePath)
	if _, err := c.Exec(fmt.Sprintf("mkdir -p %s", security.ShellEscape(remoteDir))); err != nil {
		return fmt.Errorf("failed to create remote directory: %w", err)
	}

	// Get stdin pipe
	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	// Start scp command
	filename := filepath.Base(remotePath)
	go func() {
		defer stdin.Close()
		// SCP protocol: C<mode> <size> <filename>\n<data>\0
		fmt.Fprintf(stdin, "C0644 %d %s\n", fileInfo.Size(), filename)
		_, _ = io.Copy(stdin, localFile)
		fmt.Fprint(stdin, "\x00")
	}()

	// Run scp
	if err := session.Run(fmt.Sprintf("scp -t %s", security.ShellEscape(remotePath))); err != nil {
		return fmt.Errorf("scp failed: %w", err)
	}

	return nil
}

// UploadContent uploads content directly to a remote file
// SECURITY: Uses base64 encoding to prevent heredoc injection attacks
func (c *Client) UploadContent(content, remotePath string) error {
	// Encode content as base64 to prevent any shell injection
	base64Content := base64.StdEncoding.EncodeToString([]byte(content))

	// Use base64 decoding on the remote side
	cmd := fmt.Sprintf("mkdir -p %s && echo '%s' | base64 -d > %s",
		security.ShellEscape(filepath.Dir(remotePath)), base64Content, security.ShellEscape(remotePath))

	result, err := c.Exec(cmd)
	if err != nil {
		return fmt.Errorf("failed to upload content: %w", err)
	}

	if result.ExitCode != 0 {
		return fmt.Errorf("failed to write file: %s", result.Stderr)
	}

	return nil
}

// DownloadFile downloads a remote file to the local filesystem
func (c *Client) DownloadFile(remotePath, localPath string) error {
	// Create local directory if needed
	localDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("failed to create local directory: %w", err)
	}

	// Read remote file content
	result, err := c.Exec(fmt.Sprintf("cat %s", security.ShellEscape(remotePath)))
	if err != nil {
		return fmt.Errorf("failed to read remote file: %w", err)
	}

	if result.ExitCode != 0 {
		return fmt.Errorf("failed to read remote file: %s", result.Stderr)
	}

	// Write to local file
	if err := os.WriteFile(localPath, []byte(result.Stdout), 0644); err != nil {
		return fmt.Errorf("failed to write local file: %w", err)
	}

	return nil
}

// UploadDirectory uploads a local directory to the remote server
func (c *Client) UploadDirectory(localDir, remoteDir string) error {
	// Create remote directory
	if _, err := c.Exec(fmt.Sprintf("mkdir -p %s", security.ShellEscape(remoteDir))); err != nil {
		return fmt.Errorf("failed to create remote directory: %w", err)
	}

	// Walk local directory
	return filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(localDir, path)
		if err != nil {
			return err
		}

		remotePath := filepath.Join(remoteDir, relPath)

		if info.IsDir() {
			_, err := c.Exec(fmt.Sprintf("mkdir -p %s", security.ShellEscape(remotePath)))
			return err
		}

		return c.UploadFile(path, remotePath)
	})
}

// FileExists checks if a file exists on the remote server
func (c *Client) FileExists(remotePath string) (bool, error) {
	result, err := c.Exec(fmt.Sprintf("test -f %s && echo 'exists'", security.ShellEscape(remotePath)))
	if err != nil {
		return false, err
	}
	return result.Stdout == "exists\n", nil
}

// DirectoryExists checks if a directory exists on the remote server
func (c *Client) DirectoryExists(remotePath string) (bool, error) {
	result, err := c.Exec(fmt.Sprintf("test -d %s && echo 'exists'", security.ShellEscape(remotePath)))
	if err != nil {
		return false, err
	}
	return result.Stdout == "exists\n", nil
}

// RemoveFile removes a file from the remote server
func (c *Client) RemoveFile(remotePath string) error {
	result, err := c.Exec(fmt.Sprintf("rm -f %s", security.ShellEscape(remotePath)))
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to remove file: %s", result.Stderr)
	}
	return nil
}

// RemoveDirectory removes a directory from the remote server
func (c *Client) RemoveDirectory(remotePath string) error {
	result, err := c.Exec(fmt.Sprintf("rm -rf %s", security.ShellEscape(remotePath)))
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to remove directory: %s", result.Stderr)
	}
	return nil
}

