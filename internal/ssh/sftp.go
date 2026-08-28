package ssh

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
)

// ProgressFunc receives upload progress: bytes written so far and the total.
type ProgressFunc func(written, total int64)

// UploadFile transfers a local file to the server over the existing SSH
// connection (pure-Go SFTP: no scp binary, no extra SSH handshake, the
// FRANKENDEPLOY_SSH_KEY/known-hosts configuration is honored by construction).
// The context cancels the transfer between chunks.
func (c *Client) UploadFile(ctx context.Context, localPath, remotePath string, progress ProgressFunc) error {
	local, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", localPath, err)
	}
	defer local.Close()

	info, err := local.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", localPath, err)
	}

	sftpClient, err := sftp.NewClient(c.client)
	if err != nil {
		return fmt.Errorf("failed to open SFTP session: %w", err)
	}
	defer sftpClient.Close()

	remote, err := sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file %s: %w", remotePath, err)
	}
	defer remote.Close()

	if err := copyWithProgress(ctx, remote, local, info.Size(), progress); err != nil {
		return fmt.Errorf("upload of %s failed: %w", localPath, err)
	}
	return nil
}

// UploadDirOptions configures UploadDir.
type UploadDirOptions struct {
	// Exclude lists path prefixes (relative to the source dir, slash
	// separated) that are skipped entirely.
	Exclude []string
	// Progress is called after each uploaded file with the running count.
	Progress func(uploaded int, currentFile string)
}

// UploadDir recursively uploads a local directory into remoteDir (which must
// exist) over the existing SSH connection, preserving file permission bits.
// Returns the number of uploaded files.
func (c *Client) UploadDir(ctx context.Context, localDir, remoteDir string, opts UploadDirOptions) (int, error) {
	sftpClient, err := sftp.NewClient(c.client)
	if err != nil {
		return 0, fmt.Errorf("failed to open SFTP session: %w", err)
	}
	defer sftpClient.Close()

	uploaded := 0
	err = filepath.Walk(localDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		rel, err := filepath.Rel(localDir, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		relSlash := filepath.ToSlash(rel)

		if isExcluded(relSlash, opts.Exclude) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		remotePath := path.Join(remoteDir, relSlash)

		if info.IsDir() {
			if err := sftpClient.MkdirAll(remotePath); err != nil {
				return fmt.Errorf("failed to create remote dir %s: %w", remotePath, err)
			}
			return nil
		}
		// Skip anything that is not a regular file (sockets, symlinks out of
		// tree...) — same behavior a docker build context expects
		if !info.Mode().IsRegular() {
			return nil
		}

		local, err := os.Open(p)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", p, err)
		}
		remote, err := sftpClient.Create(remotePath)
		if err != nil {
			local.Close()
			return fmt.Errorf("failed to create remote file %s: %w", remotePath, err)
		}
		_, copyErr := io.Copy(remote, local)
		closeErr := remote.Close()
		local.Close()
		if copyErr != nil {
			return fmt.Errorf("upload of %s failed: %w", relSlash, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("upload of %s failed: %w", relSlash, closeErr)
		}
		// Preserve permission bits (bin/console must stay executable)
		if err := sftpClient.Chmod(remotePath, info.Mode().Perm()); err != nil {
			return fmt.Errorf("failed to chmod %s: %w", remotePath, err)
		}

		uploaded++
		if opts.Progress != nil {
			opts.Progress(uploaded, relSlash)
		}
		return nil
	})
	if err != nil {
		return uploaded, err
	}
	return uploaded, nil
}

// isExcluded reports whether a slash-separated relative path matches an
// exclude entry (exact match or contained in an excluded directory).
func isExcluded(relSlash string, excludes []string) bool {
	for _, excl := range excludes {
		if relSlash == excl || strings.HasPrefix(relSlash, excl+"/") {
			return true
		}
	}
	return false
}

// copyWithProgress copies src to dst in chunks, checking ctx between chunks
// and reporting progress.
func copyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, total int64, progress ProgressFunc) error {
	buf := make([]byte, 256*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			written += int64(n)
			if progress != nil {
				progress(written, total)
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}
