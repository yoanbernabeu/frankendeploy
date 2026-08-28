package deploy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/constants"
	"github.com/yoanbernabeu/frankendeploy/internal/generator"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

// DBReadinessAttempts is how many 1s attempts the readiness wait makes.
var DBReadinessAttempts = 30

// DeployManagedDatabase ensures the managed database container exists and
// returns its DATABASE_URL.
//
// Credential safety: postgres/mysql/mariadb images only apply the *_PASSWORD
// env vars when the data directory is EMPTY. Recreating the container with
// freshly generated credentials while the data volume survives (VPS reboot,
// container crash, lost .db_credentials) would save a DATABASE_URL that the
// database never accepted — auth errors everywhere. So:
//   - existing container (running or stopped) + credentials file → reuse
//     (start the container if needed);
//   - data volume exists but the credentials file is gone → fail explicitly
//     instead of silently generating a password the database will ignore;
//   - nothing exists → create container + volume with fresh credentials.
func DeployManagedDatabase(ctx context.Context, client ssh.Executor, cfg *config.ProjectConfig, appPath string, log Logger) (string, error) {
	if log == nil {
		log = NopLogger{}
	}

	dbContainerName := fmt.Sprintf("%s-db", cfg.Name)
	dbVolumeName := fmt.Sprintf("%s-data", dbContainerName)
	dbName := strings.ReplaceAll(cfg.Name, "-", "_")
	credentialsFile := filepath.Join(appPath, "shared", ".db_credentials")

	// Get driver info from registry (single source of truth)
	info, err := generator.GetDBDriverInfo(cfg.Database.Driver)
	if err != nil {
		return "", fmt.Errorf("unsupported database driver for managed mode: %s", cfg.Database.Driver)
	}

	// Read saved credentials (empty when the file is missing)
	savedURL := ""
	if credResult, credErr := client.Exec(ctx, fmt.Sprintf("cat %s 2>/dev/null", credentialsFile)); credErr == nil && credResult.ExitCode == 0 {
		savedURL = strings.TrimSpace(credResult.Stdout)
	}

	// Any existing container (running or not)? -a is essential: after a VPS
	// reboot without --restart, or a crash, the container exists but is
	// stopped — recreating it with new credentials would corrupt the setup.
	containerExists := false
	containerRunning := false
	if result, err := client.Exec(ctx, fmt.Sprintf("docker ps -aq -f name=^%s$", dbContainerName)); err == nil {
		containerExists = strings.TrimSpace(result.Stdout) != ""
	}
	if containerExists {
		if result, err := client.Exec(ctx, fmt.Sprintf("docker ps -q -f name=^%s$", dbContainerName)); err == nil {
			containerRunning = strings.TrimSpace(result.Stdout) != ""
		}
	}

	if containerExists && savedURL != "" {
		if !containerRunning {
			log.Info("Database container is stopped, starting it...")
			result, err := client.Exec(ctx, fmt.Sprintf("docker start %s", dbContainerName))
			if err != nil {
				return "", fmt.Errorf("failed to start existing database container: %w", err)
			}
			if err := result.Err(); err != nil {
				return "", fmt.Errorf("failed to start existing database container: %w", err)
			}
			if err := waitForDatabase(ctx, client, info, dbContainerName, savedURL); err != nil {
				return "", err
			}
		}
		return savedURL, nil
	}

	// Data volume without credentials: generating a new password would be
	// ignored by the database (non-empty datadir) while the saved URL claims
	// it — fail explicitly instead.
	volumeExists := false
	if result, err := client.Exec(ctx, fmt.Sprintf("docker volume ls -q -f name=^%s$", dbVolumeName)); err == nil {
		volumeExists = strings.TrimSpace(result.Stdout) != ""
	}
	if volumeExists && savedURL == "" {
		return "", fmt.Errorf("database volume %s exists but %s is missing: the existing data keeps its old password, so regenerating credentials would break authentication.\n"+
			"Either restore the credentials file, or remove the old data to start fresh:\n"+
			"  docker rm -f %s && docker volume rm %s",
			dbVolumeName, credentialsFile, dbContainerName, dbVolumeName)
	}
	if containerExists && savedURL == "" {
		// Container without volume and without credentials: safe to recreate
		log.Warning("Existing database container has no saved credentials, recreating it...")
	}

	// Fresh setup: generate credentials
	dbUser := cfg.Name
	dbPassword, err := generateRandomPassword(24)
	if err != nil {
		return "", err
	}

	// Use configured version or registry default
	version := cfg.Database.Version
	if version == "" {
		version = info.DefaultVersion
	}

	dockerImage := info.FullImage(version)
	dockerEnv := info.BuildEnvArgs(
		security.ShellEscape(dbUser),
		security.ShellEscape(dbPassword),
		security.ShellEscape(dbName),
	)
	databaseURL := info.BuildDatabaseURL(dbUser, dbPassword, dbContainerName, dbName, version)

	// Remove any leftover container before recreation
	_, _ = client.Exec(ctx, fmt.Sprintf("docker stop %s 2>/dev/null || true", dbContainerName))
	_, _ = client.Exec(ctx, fmt.Sprintf("docker rm %s 2>/dev/null || true", dbContainerName))

	// Create database container with persistent volume
	dbRunCmd := fmt.Sprintf(`docker run -d --name %s \
		--network %s \
		--restart unless-stopped \
		%s \
		%s \
		-v %s:%s \
		%s`,
		dbContainerName,
		constants.NetworkName,
		constants.DockerLogOptions,
		dockerEnv,
		dbVolumeName,
		info.DataVolumePath,
		dockerImage)

	result, err := client.Exec(ctx, dbRunCmd)
	if err != nil {
		return "", fmt.Errorf("failed to start database container: %w", err)
	}
	if err := result.Err(); err != nil {
		return "", fmt.Errorf("failed to start database container: %w", err)
	}

	// Save credentials to file
	if _, err := client.Exec(ctx, fmt.Sprintf("echo %s > %s", security.ShellEscape(databaseURL), credentialsFile)); err != nil {
		return "", fmt.Errorf("failed to save database credentials: %w", err)
	}
	if _, err := client.Exec(ctx, fmt.Sprintf("chmod 600 %s", credentialsFile)); err != nil {
		log.Warning("Could not set permissions on credentials file: %v", err)
	}

	log.Info("Waiting for database to be ready...")
	if err := waitForDatabase(ctx, client, info, dbContainerName, databaseURL); err != nil {
		return "", err
	}

	return databaseURL, nil
}

// waitForDatabase polls the driver health command until the database accepts
// connections. A database that never becomes ready fails the deploy
// explicitly (the silent continue meant migrations exploded later with a
// confusing connection error).
func waitForDatabase(ctx context.Context, client ssh.Executor, info generator.DBDriverInfo, containerName, databaseURL string) error {
	user, password, _, err := parseDatabaseURL(databaseURL)
	if err != nil {
		return fmt.Errorf("cannot parse database URL for readiness check: %w", err)
	}
	for i := 0; i < DBReadinessAttempts; i++ {
		healthCmd := info.BuildHealthCmd(
			containerName,
			security.ShellEscape(user),
			security.ShellEscape(password),
		)
		checkResult, _ := client.Exec(ctx, healthCmd)
		if checkResult != nil && checkResult.ExitCode == 0 {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("database %s did not become ready after %d seconds — check its logs: docker logs %s",
		containerName, DBReadinessAttempts, containerName)
}

// generateRandomPassword generates a secure random password.
func generateRandomPassword(length int) (string, error) {
	b := make([]byte, length/2)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random password: %w", err)
	}
	return hex.EncodeToString(b), nil
}
