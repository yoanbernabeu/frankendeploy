package deploy

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/constants"
	"github.com/yoanbernabeu/frankendeploy/internal/generator"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

// parseDatabaseURL extracts the credentials from a Symfony DATABASE_URL
// (scheme://user:password@host:port/dbname?params).
func parseDatabaseURL(databaseURL string) (user, password, dbName string, err error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid DATABASE_URL: %w", err)
	}
	if parsed.User == nil || parsed.User.Username() == "" {
		return "", "", "", fmt.Errorf("DATABASE_URL has no user")
	}
	dbName = strings.TrimPrefix(parsed.Path, "/")
	if dbName == "" {
		return "", "", "", fmt.Errorf("DATABASE_URL has no database name")
	}
	password, _ = parsed.User.Password()
	return parsed.User.Username(), password, dbName, nil
}

// BackupManagedDatabase dumps the app's managed database container into
// shared/backups/ before a migration runs. The dump is gzipped, chmod 600,
// verified non-empty, and old backups are pruned to the keep_releases count.
// Returns the remote path of the backup file.
func BackupManagedDatabase(ctx context.Context, client ssh.Executor, cfg *config.ProjectConfig, databaseURL, tag string) (string, error) {
	info, err := generator.GetDBDriverInfo(cfg.Database.Driver)
	if err != nil {
		return "", fmt.Errorf("unsupported database driver for backup: %s", cfg.Database.Driver)
	}

	user, password, dbName, err := parseDatabaseURL(databaseURL)
	if err != nil {
		return "", err
	}

	dbContainerName := fmt.Sprintf("%s-db", cfg.Name)
	backupDir := filepath.Join(constants.AppSharedPath(cfg.Name), "backups")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("%s-%s.sql.gz", cfg.Database.Driver, tag))

	if _, err := client.Exec(ctx, fmt.Sprintf("mkdir -p %s && chmod 700 %s", backupDir, backupDir)); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	dumpCmd := info.BuildDumpCmd(
		dbContainerName,
		security.ShellEscape(user),
		security.ShellEscape(password),
		security.ShellEscape(dbName),
	)
	// pipefail: a failing dump must not be masked by a succeeding gzip
	fullCmd := fmt.Sprintf("set -o pipefail; %s | gzip > %s", dumpCmd, backupPath)
	result, err := client.Exec(ctx, fullCmd)
	if err != nil {
		return "", fmt.Errorf("database dump failed: %w", err)
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("database dump failed: %s", strings.TrimSpace(result.Stderr))
	}

	// An empty file means the dump silently produced nothing: refuse it
	// rather than pretend a safety net exists.
	checkResult, err := client.Exec(ctx, fmt.Sprintf("test -s %s", backupPath))
	if err != nil || checkResult == nil || checkResult.ExitCode != 0 {
		return "", fmt.Errorf("database dump produced an empty file at %s", backupPath)
	}

	if _, err := client.Exec(ctx, fmt.Sprintf("chmod 600 %s", backupPath)); err != nil {
		return "", fmt.Errorf("failed to set backup permissions: %w", err)
	}

	// Retention aligned with keep_releases: newest first, delete the rest
	keep := cfg.Deploy.KeepReleases
	if keep <= 0 {
		keep = constants.DefaultKeepReleases
	}
	pruneCmd := fmt.Sprintf("ls -1t %s/*.sql.gz 2>/dev/null | tail -n +%d | xargs -r rm -f", backupDir, keep+1)
	if _, err := client.Exec(ctx, pruneCmd); err != nil {
		// Non-critical: a failed prune never blocks a deploy
		_ = err
	}

	return backupPath, nil
}
