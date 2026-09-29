package deploy

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/constants"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

// image is the MongoDB community server image used for the managed mongodb
// service. This file is scoped to MongoDB, so internal names are unprefixed;
// the image string keeps its full 'mongodb' spelling.
const image = "mongodb/mongodb-community-server:8.0"

// replicaSetName is the single-node replica set name.
const replicaSetName = "rs0"

// mongoAttempts bounds each mongodb readiness/PRIMARY wait. Higher than the SQL
// readiness budget: a cold mongod plus a single-node election can exceed 30s.
var mongoAttempts = 90

// credentialsFile returns the path where the managed mongodb MONGODB_URI is
// persisted for reuse across deploys, rollbacks and reloads.
func credentialsFile(appPath string) string {
	return filepath.Join(appPath, "shared", ".mongo_credentials")
}

// DeployMongoDBService ensures the managed mongodb container exists (when
// mongodb.managed is true) and returns the MONGODB_URI the app must use.
// Container and volume names are per-app (<app>-mongodb / <app>-mongodb-data)
// so two apps on the same server never collide. Credentials are persisted only
// once the single-node replica set is PRIMARY, so a failed init is retried on
// the next deploy.
func DeployMongoDBService(ctx context.Context, client ssh.Executor, cfg *config.ProjectConfig, appPath string, log Logger) (string, error) {
	if log == nil {
		log = NopLogger{}
	}
	dbName := strings.ReplaceAll(cfg.Name, "-", "_")
	containerName := fmt.Sprintf("%s-mongodb", cfg.Name)
	volumeName := fmt.Sprintf("%s-mongodb-data", cfg.Name)
	memberHost := containerName + ":27017"
	credFile := credentialsFile(appPath)

	savedURL := ""
	if result, err := client.Exec(ctx, fmt.Sprintf("cat %s 2>/dev/null", credFile)); err == nil && result != nil && result.ExitCode == 0 {
		savedURL = strings.TrimSpace(result.Stdout)
	}

	containerExists := false
	containerRunning := false
	if result, err := client.Exec(ctx, fmt.Sprintf("docker ps -aq -f name=^%s$", containerName)); err == nil && result != nil {
		containerExists = strings.TrimSpace(result.Stdout) != ""
	}
	if containerExists {
		if result, err := client.Exec(ctx, fmt.Sprintf("docker ps -q -f name=^%s$", containerName)); err == nil && result != nil {
			containerRunning = strings.TrimSpace(result.Stdout) != ""
		}
	}

	if containerExists && savedURL != "" {
		if !containerRunning {
			log.Info("mongodb container is stopped, starting it...")
			if _, err := client.Exec(ctx, fmt.Sprintf("docker start %s", containerName)); err != nil {
				return "", fmt.Errorf("failed to start mongodb container: %w", err)
			}
		}
		// A previous rs.initiate() may never have completed (interrupted first
		// start): make sure the single-node replica set is PRIMARY before reuse.
		if err := ensureReplicaSetPrimary(ctx, client, containerName, memberHost, savedURL); err != nil {
			return "", err
		}
		return savedURL, nil
	}

	// Data volume without credentials: generating a new password would be
	// ignored by the server (non-empty datadir) while the saved URI claims it
	// — fail explicitly instead.
	volumeExists := false
	if result, err := client.Exec(ctx, fmt.Sprintf("docker volume ls -q -f name=^%s$", volumeName)); err == nil {
		volumeExists = strings.TrimSpace(result.Stdout) != ""
	}
	if volumeExists && savedURL == "" {
		return "", fmt.Errorf("mongodb volume %s exists but %s is missing: the existing data keeps its old credentials, so regenerating a password would break authentication.\n"+
			"Either restore the credentials file, or remove the old data to start fresh:\n"+
			"  docker rm -f %s && docker volume rm %s",
			volumeName, credFile, containerName, volumeName)
	}

	// Fresh setup: generate credentials
	user := cfg.Name
	password, err := generateRandomPassword(24)
	if err != nil {
		return "", err
	}
	// Single-node replica set: the driver connects with replicaSet=rs0.
	databaseURL := fmt.Sprintf("mongodb://%s:%s@%s:27017/%s?authSource=admin&replicaSet=%s",
		user, password, containerName, dbName, replicaSetName)

	// Remove any leftover container before recreation
	_, _ = client.Exec(ctx, fmt.Sprintf("docker stop %s 2>/dev/null || true", containerName))
	_, _ = client.Exec(ctx, fmt.Sprintf("docker rm %s 2>/dev/null || true", containerName))

	envArgs := fmt.Sprintf("-e MONGO_INITDB_ROOT_USERNAME=%s -e MONGO_INITDB_ROOT_PASSWORD=%s -e MONGO_INITDB_DATABASE=%s",
		security.ShellEscape(user), security.ShellEscape(password), security.ShellEscape(dbName))
	runCmd := fmt.Sprintf(`docker run -d --name %s \
		--network %s \
		--restart unless-stopped \
		%s \
		%s \
		-v %s:%s \
		%s --replSet %s`,
		containerName,
		constants.AppNetworkName(cfg.Name),
		constants.DockerLogOptions,
		envArgs,
		volumeName,
		"/data/db",
		image,
		replicaSetName)
	if result, err := client.Exec(ctx, runCmd); err != nil {
		return "", fmt.Errorf("failed to start mongodb container: %w", err)
	} else if err := result.Err(); err != nil {
		return "", fmt.Errorf("failed to start mongodb container: %w", err)
	}

	log.Info("Waiting for mongodb to be ready...")
	if err := waitForAccepting(ctx, client, containerName, user, password); err != nil {
		return "", err
	}

	// Persist credentials once the container accepts the root user: the URI is
	// valid even if the replica-set init below is interrupted. On a later deploy
	// the reuse branch runs ensureReplicaSetPrimary to finish it, so a failed
	// init is retried instead of deadlocking on the orphaned-volume guard.
	if _, err := client.Exec(ctx, fmt.Sprintf("echo %s > %s", security.ShellEscape(databaseURL), credFile)); err != nil {
		return "", fmt.Errorf("failed to save mongodb credentials: %w", err)
	}
	if _, err := client.Exec(ctx, fmt.Sprintf("chmod 600 %s", credFile)); err != nil {
		log.Warning("Could not set permissions on credentials file: %v", err)
	}

	// Turn the node into a single-node replica set and wait for PRIMARY.
	if err := initiateReplicaSet(ctx, client, containerName, memberHost, user, password); err != nil {
		return "", err
	}

	return databaseURL, nil
}

// waitForAccepting polls mongosh until the container accepts an authenticated
// connection (the root user is created by MONGO_INITDB_ROOT_*).
func waitForAccepting(ctx context.Context, client ssh.Executor, containerName, user, password string) error {
	for i := 0; i < mongoAttempts; i++ {
		pingCmd := fmt.Sprintf("docker exec %s mongosh -u %s -p %s --authenticationDatabase admin --quiet --eval \"db.adminCommand({ping:1}).ok\"",
			containerName, security.ShellEscape(user), security.ShellEscape(password))
		checkResult, _ := client.Exec(ctx, pingCmd)
		if checkResult != nil && checkResult.ExitCode == 0 {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("mongodb %s did not become ready after %d seconds — check its logs: docker logs %s",
		containerName, mongoAttempts, containerName)
}

// initiateReplicaSet turns the fresh mongodb node into a single-node replica
// set whose member host is explicit (so the container UUID never leaks into
// the member list), then waits for PRIMARY.
func initiateReplicaSet(ctx context.Context, client ssh.Executor, containerName, memberHost, user, password string) error {
	initCmd := fmt.Sprintf("docker exec %s mongosh -u %s -p %s --authenticationDatabase admin --quiet --eval 'rs.initiate({_id:\"%s\", members:[{_id:0, host:\"%s\"}]})'",
		containerName, security.ShellEscape(user), security.ShellEscape(password), replicaSetName, memberHost)
	if result, err := client.Exec(ctx, initCmd); err != nil {
		return fmt.Errorf("failed to initiate mongodb replica set: %w", err)
	} else if err := result.Err(); err != nil {
		// An exit code other than 0 (e.g. AlreadyInitialized, auth failure) is
		// surfaced rather than swallowed until the PRIMARY wait times out.
		return fmt.Errorf("failed to initiate mongodb replica set: %w", err)
	}
	return waitForPrimary(ctx, client, containerName, user, password)
}

// ensureReplicaSetPrimary makes sure an existing managed mongodb node is a
// PRIMARY; a previously failed init is retried here.
func ensureReplicaSetPrimary(ctx context.Context, client ssh.Executor, containerName, memberHost, databaseURL string) error {
	user, password, _, err := parseDatabaseURL(databaseURL)
	if err != nil {
		// The reused URI does not parse for an auth check; let the app surface
		// any problem rather than block the deploy.
		return nil
	}
	if isPrimary(ctx, client, containerName, user, password) {
		return nil
	}
	return initiateReplicaSet(ctx, client, containerName, memberHost, user, password)
}

// waitForPrimary polls rs.status() until the single member is PRIMARY.
func waitForPrimary(ctx context.Context, client ssh.Executor, containerName, user, password string) error {
	for i := 0; i < mongoAttempts; i++ {
		if isPrimary(ctx, client, containerName, user, password) {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("mongodb replica set did not become PRIMARY after %d seconds — check its logs: docker logs %s",
		mongoAttempts, containerName)
}

// isPrimary reports whether the node reports its single member as PRIMARY.
func isPrimary(ctx context.Context, client ssh.Executor, containerName, user, password string) bool {
	statusCmd := fmt.Sprintf("docker exec %s mongosh -u %s -p %s --authenticationDatabase admin --quiet --eval 'rs.status().members[0].stateStr'",
		containerName, security.ShellEscape(user), security.ShellEscape(password))
	checkResult, _ := client.Exec(ctx, statusCmd)
	return checkResult != nil && checkResult.ExitCode == 0 && strings.Contains(checkResult.Stdout, "PRIMARY")
}

// readSavedMongoURL returns the MONGODB_URI persisted by a managed mongodb
// deploy (shared/.mongo_credentials). Empty when absent.
func readSavedMongoURL(ctx context.Context, client ssh.Executor, appPath string) string {
	result, err := client.Exec(ctx, fmt.Sprintf("cat %s 2>/dev/null", credentialsFile(appPath)))
	if err != nil || result == nil || result.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

// ManagedMongoEnvVar returns a "MONGODB_URI=<uri>" docker env pair to inject
// into the app when it uses a managed mongodb service, or "" otherwise (the
// caller provides an external MONGODB_URI).
func ManagedMongoEnvVar(ctx context.Context, client ssh.Executor, cfg *config.ProjectConfig, appPath string) string {
	if !cfg.MongoDB.Enabled || !cfg.MongoDB.Managed {
		return ""
	}
	uri := readSavedMongoURL(ctx, client, appPath)
	if uri == "" {
		return ""
	}
	return "MONGODB_URI=" + security.ShellEscape(uri)
}
