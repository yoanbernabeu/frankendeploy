package cmd

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/frankendeploy/internal/caddy"
	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/constants"
	"github.com/yoanbernabeu/frankendeploy/internal/deploy"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

var deployCmd = &cobra.Command{
	Use:   "deploy [server]",
	Short: "Deploy application to a server",
	Long: `Deploys the application to the specified server.

The deployment process:
1. Builds Docker image locally
2. Pushes image to server
3. Starts new container
4. Runs health checks
5. Switches traffic to new version
6. Cleans up old releases

CI/CD: If no server is specified, FRANKENDEPLOY_SERVER environment variable is used.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDeploy,
}

var (
	deployTag             string
	deployForce           bool
	deployNoBuild         bool
	deployRemoteBuild     bool
	deployNoRemoteBuild   bool
	deploySkipEnvCheck    bool
	deploySkipHealthcheck bool
)

func init() {
	rootCmd.AddCommand(deployCmd)
	deployCmd.Flags().StringVarP(&deployTag, "tag", "t", "", "Image tag (default: timestamp)")
	deployCmd.Flags().BoolVarP(&deployForce, "force", "f", false, "Skip env pre-flight and continue on hook or health check failures")
	deployCmd.Flags().BoolVar(&deploySkipEnvCheck, "skip-env-check", false, "Skip the pre-flight environment variables check")
	deployCmd.Flags().BoolVar(&deploySkipHealthcheck, "skip-healthcheck", false, "Skip the health check on the new container (traffic switches unverified)")
	deployCmd.Flags().BoolVar(&deployNoBuild, "no-build", false, "Skip image build (use existing image)")
	deployCmd.Flags().BoolVar(&deployRemoteBuild, "remote-build", false, "Build image on the server (recommended for cross-architecture)")
	deployCmd.Flags().BoolVar(&deployNoRemoteBuild, "no-remote-build", false, "Force local build (ignore saved preference)")
}

func runDeploy(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Get server name from args or environment variable
	var serverName string
	if len(args) > 0 {
		serverName = args[0]
	} else if envServer := os.Getenv("FRANKENDEPLOY_SERVER"); envServer != "" {
		serverName = envServer
		PrintInfo("Using server from FRANKENDEPLOY_SERVER: %s", serverName)
	} else {
		return fmt.Errorf("no server specified. Usage: frankendeploy deploy <server> or set FRANKENDEPLOY_SERVER")
	}

	// Step 1: Connect to server (validates name, loads config, applies SSHTimeout)
	conn, err := ConnectToServer(serverName)
	if err != nil {
		return fmt.Errorf("%w\n\nRun 'frankendeploy doctor %s' to diagnose the server setup", err, serverName)
	}
	defer conn.Client.Close()

	client := conn.Client
	projectCfg := conn.Project
	serverCfg := conn.Server
	globalCfg := conn.Global

	deployStart := time.Now()
	PrintInfo("Deploying %s to %s...", projectCfg.Name, serverName)
	PrintSuccess("Connected to %s", serverCfg.Host)

	// Generate tag if not provided
	if deployTag == "" {
		deployTag = time.Now().Format("20060102-150405")
	} else {
		// Validate user-provided tag
		if err := security.ValidateRelease(deployTag); err != nil {
			return fmt.Errorf("invalid deploy tag: %w", err)
		}
	}

	imageName := fmt.Sprintf("%s:%s", projectCfg.Name, deployTag)
	remoteAppPath := constants.AppBasePath(projectCfg.Name)

	// Step 1a: Check architecture compatibility
	useRemoteBuild, err := checkArchitectureMismatch(ctx, client, serverCfg, globalCfg, serverName)
	if err != nil {
		return err
	}
	if useRemoteBuild && !deployRemoteBuild {
		deployRemoteBuild = true
	}

	// Step 1b: Pre-flight environment check
	if deployForce || deploySkipEnvCheck {
		PrintWarning("Pre-flight environment check skipped (%s)", skipFlagName(deploySkipEnvCheck, "--skip-env-check"))
	} else {
		PrintInfo("Running pre-flight checks...")
		if err := runEnvPreflightCheck(ctx, client, projectCfg, serverName); err != nil {
			return err
		}
		PrintSuccess("Pre-flight checks passed")
	}

	// Step 2: Ensure Docker artifacts exist (novice flow: init → deploy without build)
	if deployRemoteBuild || !deployNoBuild {
		if err := ensureDockerArtifacts(projectCfg); err != nil {
			return err
		}
	}

	if deployRemoteBuild {
		// Remote build: transfer source code and build on server
		PrintInfo("Transferring source code to server...")
		if err := transferSourceCode(ctx, client, projectCfg.Name, remoteAppPath); err != nil {
			return fmt.Errorf("transfer failed: %w\n\nRun 'frankendeploy doctor %s' to diagnose the server setup", err, serverName)
		}
		PrintSuccess("Source code transferred")

		PrintInfo("Building Docker image on server...")
		if err := buildDockerImageRemote(ctx, client, imageName, remoteAppPath); err != nil {
			return fmt.Errorf("remote build failed: %w\n\nRun 'frankendeploy doctor %s' to diagnose the server setup", err, serverName)
		}
		PrintSuccess("Image built: %s", imageName)
	} else {
		// Local build: build locally and transfer image
		if !deployNoBuild {
			platform := buildPlatformForServer(ctx, client)
			PrintInfo("Building Docker image locally (%s)...", platform)
			if err := buildDockerImage(imageName, platform); err != nil {
				return fmt.Errorf("build failed: %w", err)
			}
			PrintSuccess("Image built: %s", imageName)
		}

		PrintInfo("Transferring image to server...")
		if err := transferImage(ctx, client, imageName); err != nil {
			return fmt.Errorf("transfer failed: %w", err)
		}
		PrintSuccess("Image transferred")
	}

	// Step 3b: Deploy managed database if configured
	var databaseURL string
	if projectCfg.Database.Driver != "" && projectCfg.Database.IsManaged() {
		PrintInfo("Setting up managed database...")
		var err error
		databaseURL, err = deploy.DeployManagedDatabase(ctx, client, projectCfg, remoteAppPath, cmdLogger{})
		if err != nil {
			return fmt.Errorf("database setup failed: %w", err)
		}
		PrintSuccess("Database ready: %s", projectCfg.Database.Driver)
	}

	// Blue-green deployment: start new container with temp name, health check, then swap
	state := deploy.NewDeployState(projectCfg.Name)

	// Steps 4-11: blue-green orchestration (extracted to internal/deploy,
	// tested per failure scenario in orchestrator_test.go)
	steps := deploy.Steps{
		PrepareRelease: func() error {
			return prepareRelease(ctx, client, projectCfg, remoteAppPath, deployTag)
		},
		OldContainerExists: func() bool {
			if oldResult, err := client.Exec(ctx, fmt.Sprintf("docker ps -q -f name=^%s$", projectCfg.Name)); err == nil && oldResult != nil {
				return strings.TrimSpace(oldResult.Stdout) != ""
			}
			return false
		},
		StartNewContainer: func() error {
			return startNewContainer(ctx, client, projectCfg, imageName, remoteAppPath, deployTag, databaseURL, state.TempContainerName)
		},
		BackupDatabase: func() (string, error) {
			return deploy.BackupManagedDatabase(ctx, client, projectCfg, databaseURL, deployTag)
		},
		RunPreDeployHooks: func() error {
			return runDeployHooks(ctx, client, state.TempContainerName, projectCfg.Deploy.Hooks.PreDeploy)
		},
		CheckMigrationState: func() {
			checkAndWarnMigrationState(ctx, client, state.TempContainerName)
		},
		HealthCheck: func() error {
			return runHealthCheckOnContainer(ctx, client, projectCfg, state.TempContainerName)
		},
		ShowContainerLogs: func() {
			showContainerLogs(ctx, client, state.TempContainerName)
		},
		SwapContainers: func(oldExists bool) error {
			return swapContainers(ctx, client, projectCfg.Name, remoteAppPath, deployTag, state.TempContainerName, oldExists)
		},
		DeployWorkers: func() error {
			return deployMessengerWorkers(ctx, client, projectCfg, imageName, remoteAppPath, databaseURL)
		},
		RunPostDeployHooks: func() error {
			return runDeployHooks(ctx, client, projectCfg.Name, projectCfg.Deploy.Hooks.PostDeploy)
		},
		CaddyAppConfigExists: func() bool {
			return caddyAppConfigExists(ctx, client, projectCfg.Name)
		},
		UpdateCaddy: func() error {
			return updateCaddyConfig(ctx, client, projectCfg)
		},
		Cleanup: func() {
			cleanupOldReleases(ctx, client, remoteAppPath, projectCfg.Deploy.KeepReleases)
			// Prune images whose tag left the retention window: each deploy
			// leaves a full image behind, and a small VPS fills up after a few
			// dozen deploys
			if removed, err := deploy.PruneOldImages(ctx, client, projectCfg.Name, remoteAppPath); err != nil {
				PrintVerbose("Could not prune old images: %v", err)
			} else if len(removed) > 0 {
				PrintInfo("Removed %d old image(s): %s", len(removed), strings.Join(removed, ", "))
			}
		},
		RollbackNewContainer: func(st *deploy.DeployState) {
			rollbackNewContainer(ctx, client, st)
		},
		WarnMigrationRollback: warnDatabaseMigrationRollback,
	}

	opts := deploy.Options{
		Force:              deployForce,
		SkipHealthcheck:    deploySkipHealthcheck,
		HasPreDeployHooks:  len(projectCfg.Deploy.Hooks.PreDeploy) > 0,
		HasMigrationHook:   deploy.HasMigrationHook(projectCfg.Deploy.Hooks.PreDeploy),
		BackupEligible:     projectCfg.Database.IsManaged() && databaseURL != "",
		HasPostDeployHooks: len(projectCfg.Deploy.Hooks.PostDeploy) > 0,
		MessengerEnabled:   projectCfg.Messenger.Enabled,
		Domain:             projectCfg.Deploy.Domain,
		Logger:             cmdLogger{},
	}

	if err := deploy.RunPipeline(state, steps, opts); err != nil {
		return err
	}

	PrintSuccess("Deployment complete in %s!", time.Since(deployStart).Round(time.Second))
	fmt.Println()
	fmt.Printf("Application deployed: %s\n", projectCfg.Name)
	fmt.Printf("  Tag: %s\n", deployTag)
	if projectCfg.Deploy.Domain != "" {
		fmt.Printf("  URL: https://%s\n", projectCfg.Deploy.Domain)
	} else {
		fmt.Println("  URL: (no public domain configured)")
	}

	return nil
}

// buildPlatformForServer returns the docker --platform value matching the
// server's CPU architecture, so a local build always produces an image the
// server can run (e.g. arm64 Mac → arm64 VPS must NOT build amd64).
// Falls back to linux/amd64 when detection fails.
func buildPlatformForServer(ctx context.Context, client ssh.Executor) string {
	result, err := client.Exec(ctx, "uname -m")
	if err != nil || result == nil || result.Err() != nil {
		PrintWarning("Could not detect server architecture, building for linux/amd64")
		return "linux/amd64"
	}

	arch := normalizeArch(result.Stdout)
	switch arch {
	case "amd64", "arm64":
		return "linux/" + arch
	default:
		PrintWarning("Unknown server architecture %q, building for linux/amd64", arch)
		return "linux/amd64"
	}
}

func buildDockerImage(imageName, platform string) error {
	// Use buildx to cross-compile for the server's architecture
	dockerCmd := exec.Command("docker", "buildx", "build",
		"--platform", platform,
		"--target", "frankenphp_prod",
		"--load",
		"-t", imageName,
		".")
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr
	return dockerCmd.Run()
}

// sourceCodeExcludes lists what never belongs in the remote build context.
var sourceCodeExcludes = []string{".git", "node_modules", "vendor", "var", ".env.local"}

// transferImage uploads the locally built image over the existing SSH
// connection (pure-Go SFTP: no scp binary, no second SSH handshake, works on
// Windows, honors the configured key/known-hosts by construction).
func transferImage(ctx context.Context, client *ssh.Client, imageName string) error {
	// Save the image gzip-compressed: docker load reads .tar.gz natively and
	// PHP image layers compress ~3x — that is minutes saved on an average
	// uplink for a 700MB image
	base := strings.ReplaceAll(imageName, ":", "-")
	tarPath := filepath.Join(os.TempDir(), base+".tar.gz")

	if err := saveImageCompressed(imageName, tarPath); err != nil {
		return err
	}
	defer os.Remove(tarPath)

	info, err := os.Stat(tarPath)
	if err != nil {
		return fmt.Errorf("failed to stat image archive: %w", err)
	}
	totalMB := float64(info.Size()) / 1024 / 1024
	PrintInfo("Uploading image (%.1f MB compressed)...", totalMB)

	remoteTarPath := fmt.Sprintf("/tmp/%s.tar.gz", base)

	lastPercent := -10
	err = client.UploadFile(ctx, tarPath, remoteTarPath, func(written, total int64) {
		percent := int(written * 100 / total)
		// One line every 10%: informative without flooding CI logs
		if percent >= lastPercent+10 {
			lastPercent = percent - percent%10
			PrintInfo("  %d%% (%.1f/%.1f MB)", percent, float64(written)/1024/1024, totalMB)
		}
	})
	if err != nil {
		return fmt.Errorf("failed to upload image: %w", err)
	}

	// Load image on remote. The tar is removed even when the load fails: a
	// 500MB+ leftover in /tmp on every failed deploy fills the disk silently.
	result, err := client.Exec(ctx, fmt.Sprintf("docker load -i %s; status=$?; rm -f %s; exit $status", remoteTarPath, remoteTarPath))
	if err != nil {
		return fmt.Errorf("failed to load image on server: %w", err)
	}
	if err := result.Err(); err != nil {
		return fmt.Errorf("failed to load image on server: %w", err)
	}

	return nil
}

// saveImageCompressed streams `docker save` through gzip into destPath.
func saveImageCompressed(imageName, destPath string) error {
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create image archive: %w", err)
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	saveCmd := exec.Command("docker", "save", imageName)
	saveCmd.Stdout = gz
	var stderr bytes.Buffer
	saveCmd.Stderr = &stderr
	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("failed to compress image: %w", err)
	}
	return nil
}

// transferSourceCode uploads the project source into the remote build
// directory over the existing SSH connection (pure-Go SFTP, no rsync
// dependency — rsync does not exist on Windows).
func transferSourceCode(ctx context.Context, client *ssh.Client, appName, appPath string) error {
	// Create a fresh build directory on the server (equivalent to the old
	// rsync --delete into an empty target)
	buildPath := fmt.Sprintf("%s/build", appPath)
	if _, err := client.Exec(ctx, fmt.Sprintf("rm -rf %s && mkdir -p %s", buildPath, buildPath)); err != nil {
		return fmt.Errorf("failed to create build directory: %w", err)
	}

	count, err := client.UploadDir(ctx, ".", buildPath, ssh.UploadDirOptions{
		Exclude: sourceCodeExcludes,
		Progress: func(uploaded int, currentFile string) {
			PrintVerbose("  %s", currentFile)
		},
	})
	if err != nil {
		return fmt.Errorf("source transfer failed: %w", err)
	}
	PrintInfo("  %d files transferred", count)

	return nil
}

func buildDockerImageRemote(ctx context.Context, client ssh.Executor, imageName, appPath string) error {
	buildPath := fmt.Sprintf("%s/build", appPath)

	// Build Docker image on the server
	buildCmd := fmt.Sprintf("cd %s && docker build --target frankenphp_prod -t %s .", buildPath, imageName)

	result, err := client.Exec(ctx, buildCmd)
	if err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}
	if err := result.Err(); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}

	// Cleanup build directory
	if _, err := client.Exec(ctx, fmt.Sprintf("rm -rf %s", buildPath)); err != nil {
		PrintVerbose("Could not cleanup build directory: %v", err)
	}

	// Remote builds leave dangling layers behind (each rebuild orphans the
	// previous intermediate layers); prune them so they don't pile up
	if _, err := client.Exec(ctx, "docker image prune -f"); err != nil {
		PrintVerbose("Could not prune dangling images: %v", err)
	}

	return nil
}

// prepareRelease creates the release directory, shared directories and files, and fixes permissions.
func prepareRelease(ctx context.Context, client ssh.Executor, cfg *config.ProjectConfig, appPath, tag string) error {
	releasePath := filepath.Join(appPath, "releases", tag)
	sharedPath := filepath.Join(appPath, "shared")

	sharedDirs := cfg.Deploy.EffectiveSharedDirs()
	sharedFiles := cfg.Deploy.EffectiveSharedFiles()

	commands := []string{
		fmt.Sprintf("mkdir -p %s", releasePath),
		fmt.Sprintf("mkdir -p %s", sharedPath),
	}

	for _, dir := range sharedDirs {
		commands = append(commands, fmt.Sprintf("mkdir -p %s/%s", sharedPath, dir))
	}

	for _, file := range sharedFiles {
		dirPath := filepath.Dir(file)
		if dirPath != "." {
			commands = append(commands, fmt.Sprintf("mkdir -p %s/%s", sharedPath, dirPath))
		}
		commands = append(commands, fmt.Sprintf("touch %s/%s", sharedPath, file))
	}

	for _, command := range commands {
		PrintVerboseCommand(command)
		result, err := client.Exec(ctx, command)
		if err != nil {
			return fmt.Errorf("command failed: %w", err)
		}
		if err := result.Err(); err != nil {
			return fmt.Errorf("command failed: %w", err)
		}
	}

	fixSharedPermissions(ctx, client, sharedPath, sharedDirs, sharedFiles)
	return nil
}

// buildAppRunCommand builds the docker run command for the application
// container. It is the single source of truth shared by deploy, rollback and
// env reload: same volume mounts, env vars, non-root user and restart policy.
func buildAppRunCommand(cfg *config.ProjectConfig, imageName, appPath, databaseURL, containerName string) string {
	sharedPath := filepath.Join(appPath, "shared")

	volumeMounts := buildVolumeMounts(sharedPath, cfg.Deploy.EffectiveSharedDirs(), cfg.Deploy.EffectiveSharedFiles())

	envVars := fmt.Sprintf("-e SERVER_NAME=:%s -e APP_ENV=prod -e APP_DEBUG=0", constants.AppPort)
	if databaseURL != "" {
		envVars += fmt.Sprintf(" -e DATABASE_URL=%s", security.ShellEscape(databaseURL))
	}

	// Optional resource limits (validated at config load: safe to interpolate)
	limits := ""
	if cfg.Deploy.MemoryLimit != "" {
		limits += fmt.Sprintf(" --memory %s", cfg.Deploy.MemoryLimit)
	}
	if cfg.Deploy.CPULimit != "" {
		limits += fmt.Sprintf(" --cpus %s", cfg.Deploy.CPULimit)
	}

	// SECURITY: Run as non-root user with non-privileged port
	return fmt.Sprintf(`docker run -d --name %s \
		--network %s \
		--restart unless-stopped \
		--user %s \
		%s%s \
		%s \
		%s \
		%s`, containerName, constants.NetworkName, constants.ContainerUser, constants.DockerLogOptions, limits, envVars, volumeMounts, imageName)
}

// readSavedDatabaseURL reads the DATABASE_URL persisted by a managed-database
// deploy (shared/.db_credentials). Returns "" when no managed database is
// configured or the file cannot be read.
func readSavedDatabaseURL(ctx context.Context, client ssh.Executor, appPath string) string {
	credentialsFile := filepath.Join(appPath, "shared", ".db_credentials")
	result, err := client.Exec(ctx, fmt.Sprintf("cat %s 2>/dev/null", credentialsFile))
	if err != nil || result == nil || result.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

// startNewContainer starts the new version with a temporary container name.
// The old container remains running for zero-downtime deployment.
func startNewContainer(ctx context.Context, client ssh.Executor, cfg *config.ProjectConfig, imageName, appPath, tag, databaseURL, containerName string) error {
	// Remove any leftover temp container from a previous failed deploy
	forceRemoveContainer(ctx, client, containerName)

	dockerRunCmd := buildAppRunCommand(cfg, imageName, appPath, databaseURL, containerName)

	PrintVerboseCommand(dockerRunCmd)
	result, err := client.Exec(ctx, dockerRunCmd)
	if err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}
	if err := result.Err(); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	return nil
}

// swapContainers performs the zero-downtime swap: the old container is renamed
// away while still running (in-flight requests keep being served and the app
// name frees up instantly), the new container takes over the name, and only
// then is the old one stopped. If taking over the name fails, the old
// container is renamed back so the site keeps being served.
func swapContainers(ctx context.Context, client ssh.Executor, appName, appPath, tag, tempContainerName string, oldExists bool) error {
	releasePath := filepath.Join(appPath, "releases", tag)
	currentPath := filepath.Join(appPath, "current")

	if err := swapContainerNames(ctx, client, appName, tempContainerName, oldExists); err != nil {
		return err
	}

	// Update current symlink (critical step — surface any non-zero exit code)
	symlinkResult, err := client.Exec(ctx, fmt.Sprintf("ln -sfn %s %s", releasePath, currentPath))
	if err != nil {
		return fmt.Errorf("failed to update symlink: %w", err)
	}
	if err := symlinkResult.Err(); err != nil {
		return fmt.Errorf("failed to update symlink: %w", err)
	}

	// Save release marker (best-effort)
	releaseResult, err := client.Exec(ctx, fmt.Sprintf("echo '%s' > %s/release", tag, releasePath))
	if err != nil {
		PrintVerbose("Could not write release file: %v", err)
	} else if err := releaseResult.Err(); err != nil {
		PrintVerbose("Could not write release file: %v", err)
	}

	return nil
}

// swapContainerNames hands the app name over from the old container to the
// new one without downtime: the old container is renamed away while still
// running, the new one takes the name (two instant renames), and only then is
// the old one stopped. If taking over the name fails, the old container is
// renamed back so the site keeps being served.
func swapContainerNames(ctx context.Context, client ssh.Executor, appName, tempContainerName string, oldExists bool) error {
	oldName := appName + "-old"

	// Remove any stale -old leftover from a previously interrupted swap
	forceRemoveContainer(ctx, client, oldName)

	if oldExists {
		// Move the live container out of the name without stopping it. If this
		// fails, abort before touching anything: the old container keeps its
		// name and keeps serving.
		result, err := client.Exec(ctx, fmt.Sprintf("docker rename %s %s", appName, oldName))
		if err != nil {
			return fmt.Errorf("failed to rename old container: %w", err)
		}
		if err := result.Err(); err != nil {
			return fmt.Errorf("failed to rename old container: %w", err)
		}
	}

	// Rename temp container to final name
	result, err := client.Exec(ctx, fmt.Sprintf("docker rename %s %s", tempContainerName, appName))
	if err == nil {
		err = result.Err()
	}
	if err != nil {
		if oldExists {
			// Restore the old container under the app name: the site stays up.
			if restoreResult, restoreErr := client.Exec(ctx, fmt.Sprintf("docker rename %s %s", oldName, appName)); restoreErr != nil {
				PrintWarning("Could not restore old container: %v", restoreErr)
			} else if restoreErr := restoreResult.Err(); restoreErr != nil {
				PrintWarning("Could not restore old container: %v", restoreErr)
			} else {
				PrintWarning("Swap failed — old container restored, the site is still served by the previous version")
			}
		}
		return fmt.Errorf("failed to rename container: %w", err)
	}

	// Point of no return passed: the new container serves under the app name.
	// Stopping the old one is best-effort cleanup.
	if oldExists {
		stopAndRemoveContainer(ctx, client, oldName)
	}

	return nil
}

// rollbackNewContainer removes the temporary new container, leaving the old one intact.
// warnDatabaseMigrationRollback tells the user that rolling back the code
// does not roll back the database schema — the previous version now runs on
// the new schema, and the pre-migration dump is the only safety net.
func warnDatabaseMigrationRollback(situation, backupPath string) {
	PrintWarning("%s Rolling back the code may not be enough: the previous version now runs on the migrated schema.", situation)
	if backupPath != "" {
		PrintWarning("Database backup taken before the migration: %s", backupPath)
		PrintWarning("Restore it if needed, e.g.: gunzip -c <backup> | docker exec -i <app>-db psql -U <user> <db>  (or mysql for MySQL)")
	} else {
		PrintWarning("No automatic backup was taken (the database is not managed by FrankenDeploy)")
	}
	PrintWarning("Tip: write backward-compatible migrations (expand/contract) so a code rollback stays safe")
}

func rollbackNewContainer(ctx context.Context, client ssh.Executor, state *deploy.DeployState) {
	actions := state.RollbackActions()
	for _, action := range actions {
		PrintVerboseCommand(action)
		if _, err := client.Exec(ctx, action); err != nil {
			PrintVerbose("Rollback action failed: %v", err)
		}
	}
}

// containerLogTailLines is how many log lines are shown when a health check fails.
const containerLogTailLines = 50

// preHealthDelay is a variable so tests can skip the wait.
var preHealthDelay = constants.PreHealthSleep

// skipFlagName returns the flag responsible for skipping a check, for honest messages.
func skipFlagName(skipFlagSet bool, skipFlag string) string {
	if skipFlagSet {
		return skipFlag
	}
	return "--force"
}

// showContainerLogs prints the last log lines of a container so the user can
// see why a health check failed before the container is removed by rollback.
func showContainerLogs(ctx context.Context, client ssh.Executor, containerName string) {
	logs, err := deploy.ContainerLogs(ctx, client, containerName, containerLogTailLines)
	if err != nil {
		PrintWarning("Could not retrieve container logs: %v", err)
		return
	}
	if strings.TrimSpace(logs) == "" {
		PrintWarning("Container %s produced no logs", containerName)
		return
	}
	fmt.Println()
	PrintInfo("Last %d log lines from %s:", containerLogTailLines, containerName)
	fmt.Println(logs)
}

// runHealthCheckOnContainer runs a health check against a specific container name
// using the centralized HealthChecker with retries, timeout, and proper status code parsing.
func runHealthCheckOnContainer(ctx context.Context, client ssh.Executor, cfg *config.ProjectConfig, containerName string) error {
	healthPath := cfg.Deploy.HealthcheckPath
	if healthPath == "" {
		healthPath = "/"
	}

	if err := security.ValidateHealthPath(healthPath); err != nil {
		return fmt.Errorf("invalid health check path: %w", err)
	}

	// Wait for container to be ready before health checks
	time.Sleep(preHealthDelay)

	hc := deploy.NewHealthChecker(client, containerName, healthPath, constants.AppPort)
	if cfg.Deploy.HealthcheckTimeout > 0 {
		hc.SetTimeout(time.Duration(cfg.Deploy.HealthcheckTimeout) * time.Second)
	}
	if cfg.Deploy.HealthcheckRetries > 0 {
		hc.SetRetries(cfg.Deploy.HealthcheckRetries)
	}
	if cfg.Deploy.HealthcheckInterval > 0 {
		hc.SetInterval(time.Duration(cfg.Deploy.HealthcheckInterval) * time.Second)
	}

	result, err := hc.Check(ctx)
	if err != nil {
		return fmt.Errorf("health check error: %w", err)
	}
	if !result.Healthy {
		return fmt.Errorf("health check failed on %s: %s (after %d attempts)", containerName, result.Message, result.Attempts)
	}

	return nil
}

// buildVolumeMounts creates Docker volume mount arguments for shared dirs and files
func buildVolumeMounts(sharedPath string, sharedDirs, sharedFiles []string) string {
	var mounts []string

	// Mount shared directories
	for _, dir := range sharedDirs {
		mount := fmt.Sprintf("-v %s/%s:/app/%s", sharedPath, dir, dir)
		mounts = append(mounts, mount)
	}

	// Mount shared files (read-only for config files like .env.local)
	for _, file := range sharedFiles {
		// .env files should be read-only, others read-write
		mode := ""
		if strings.HasPrefix(file, ".env") {
			mode = ":ro"
		}
		mount := fmt.Sprintf("-v %s/%s:/app/%s%s", sharedPath, file, file, mode)
		mounts = append(mounts, mount)
	}

	return strings.Join(mounts, " \\\n\t\t")
}

// fixSharedPermissions ensures shared directories and files have correct ownership for container user 1000:1000
func fixSharedPermissions(ctx context.Context, client ssh.Executor, sharedPath string, sharedDirs, sharedFiles []string) {
	// Fix ownership of shared directory itself
	cmd := fmt.Sprintf("sudo chown %s %s 2>/dev/null || true", constants.ContainerUser, sharedPath)
	PrintVerboseCommand(cmd)
	if _, err := client.Exec(ctx, cmd); err != nil {
		PrintWarning("Could not fix permissions for shared path: %v", err)
	}

	// Fix ownership of shared directories (recursively for contents)
	for _, dir := range sharedDirs {
		dirPath := fmt.Sprintf("%s/%s", sharedPath, dir)
		cmd := fmt.Sprintf("sudo chown -R %s %s 2>/dev/null || true", constants.ContainerUser, dirPath)
		PrintVerboseCommand(cmd)
		result, err := client.Exec(ctx, cmd)
		if err != nil {
			PrintWarning("Could not fix permissions for %s: %v", dir, err)
		} else if result != nil && result.ExitCode != 0 {
			PrintWarning("Could not fix permissions for %s (may require manual sudo)", dir)
		}
	}

	// Fix ownership and permissions of shared files
	for _, file := range sharedFiles {
		filePath := fmt.Sprintf("%s/%s", sharedPath, file)
		// Set ownership
		cmd := fmt.Sprintf("sudo chown %s %s 2>/dev/null || true", constants.ContainerUser, filePath)
		PrintVerboseCommand(cmd)
		if _, err := client.Exec(ctx, cmd); err != nil {
			PrintWarning("Could not fix ownership for %s: %v", file, err)
		}

		// Set restrictive permissions for .env files (contains secrets)
		if strings.HasPrefix(file, ".env") || strings.Contains(file, "/.env") {
			cmd = fmt.Sprintf("sudo chmod 600 %s 2>/dev/null || true", filePath)
			PrintVerboseCommand(cmd)
			if _, err := client.Exec(ctx, cmd); err != nil {
				PrintWarning("Could not set permissions for %s: %v", file, err)
			}
		}
	}
}

// caddyAppConfigExists reports whether the app already has a Caddy config on
// the server — i.e. whether it has ever been publicly exposed.
func caddyAppConfigExists(ctx context.Context, client ssh.Executor, appName string) bool {
	result, err := client.Exec(ctx, fmt.Sprintf("test -f %s && echo yes", constants.CaddyAppConfig(appName)))
	return err == nil && result != nil && strings.TrimSpace(result.Stdout) == "yes"
}

func updateCaddyConfig(ctx context.Context, client ssh.Executor, cfg *config.ProjectConfig) error {
	domain := cfg.Deploy.Domain
	if domain == "" {
		fmt.Println()
		PrintWarning("No domain configured. The application will be accessible via container network only.")
		PrintInfo("To configure a public domain, add to frankendeploy.yaml:")
		fmt.Println()
		fmt.Println("   deploy:")
		fmt.Println("       domain: your-domain.com")
		fmt.Println()
		PrintInfo("Or run: frankendeploy init --domain your-domain.com")
		fmt.Println()
		return nil
	}

	// The reload runs inside the caddy container: verify it is up first so the
	// user gets a clear diagnosis instead of an obscure docker exec failure.
	statusResult, err := client.Exec(ctx, "docker inspect caddy --format '{{.State.Status}}' 2>/dev/null")
	if err != nil {
		return fmt.Errorf("could not check Caddy container status: %w", err)
	}
	if status := strings.TrimSpace(statusResult.Stdout); status != "running" {
		if status == "" {
			status = "not found"
		}
		return fmt.Errorf("caddy container is not running on the server (status: %s) — run 'frankendeploy server setup' first", status)
	}

	// Generate Caddy config using our generator
	caddyGen := caddy.NewConfigGenerator()
	appConfig := caddy.AppConfigFromProject(cfg, domain)
	configContent, err := caddyGen.GenerateAppConfig(appConfig)
	if err != nil {
		return fmt.Errorf("failed to generate Caddy config: %w", err)
	}

	// Write config and reload Caddy
	commands, err := caddy.WriteAppConfigCommands(cfg.Name, configContent)
	if err != nil {
		return fmt.Errorf("failed to prepare Caddy commands: %w", err)
	}
	for _, command := range commands {
		PrintVerboseCommand(command)
		result, err := client.Exec(ctx, command)
		if err != nil {
			return fmt.Errorf("command failed: %w", err)
		}
		if err := result.Err(); err != nil {
			return fmt.Errorf("command failed: %w", err)
		}
	}

	PrintSuccess("Caddy configured for %s", domain)
	return nil
}

func cleanupOldReleases(ctx context.Context, client ssh.Executor, appPath string, keepReleases int) {
	if keepReleases <= 0 {
		keepReleases = constants.DefaultKeepReleases
	}

	// List releases and remove old ones
	cleanupCmd := fmt.Sprintf(
		"cd %s/releases && ls -1t | tail -n +%d | xargs -r rm -rf",
		appPath, keepReleases+1)

	if _, err := client.Exec(ctx, cleanupCmd); err != nil {
		PrintVerbose("Could not cleanup old releases: %v", err)
	}
}

// runDeployHooks executes deployment hooks inside the container
func runDeployHooks(ctx context.Context, client ssh.Executor, containerName string, hooks []string) error {
	for _, hook := range hooks {
		// Validate hook command before execution
		if err := security.ValidateDockerCommand(hook); err != nil {
			return fmt.Errorf("invalid hook command %q: %w", hook, err)
		}
		PrintVerbose("  > %s", hook)
		// Execute hook inside the container
		cmd := fmt.Sprintf("docker exec %s %s", containerName, hook)
		result, err := client.Exec(ctx, cmd)
		if err != nil {
			return fmt.Errorf("hook failed: %w", err)
		}
		if err := result.Err(); err != nil {
			return fmt.Errorf("hook '%s' failed: %w", hook, err)
		}
	}
	return nil
}

// deployMessengerWorkers starts a Messenger worker container for the app.
// Only a single worker container is started; multi-worker scaling is not
// currently supported.
func deployMessengerWorkers(ctx context.Context, client ssh.Executor, cfg *config.ProjectConfig, imageName, appPath, databaseURL string) error {
	workerName := fmt.Sprintf("%s-worker", cfg.Name)

	// Build transports argument
	transports := cfg.Messenger.Transports
	if len(transports) == 0 {
		transports = []string{"async"}
	}
	transportsArg := strings.Join(transports, " ")

	// Get shared dirs and files (same as main container)
	sharedPath := filepath.Join(appPath, "shared")
	sharedDirs := cfg.Deploy.EffectiveSharedDirs()
	sharedFiles := cfg.Deploy.EffectiveSharedFiles()

	// Build volume mounts (same as main container)
	volumeMounts := buildVolumeMounts(sharedPath, sharedDirs, sharedFiles)

	// Build environment variables
	envVars := "-e APP_ENV=prod -e APP_DEBUG=0"
	if databaseURL != "" {
		envVars += fmt.Sprintf(" -e DATABASE_URL=%s", security.ShellEscape(databaseURL))
	}

	// Stop existing workers
	stopAndRemoveContainer(ctx, client, workerName)

	// Start worker container with messenger:consume command
	// SECURITY: Run as non-root user
	workerCmd := fmt.Sprintf(`docker run -d --name %s \
		--network %s \
		--restart unless-stopped \
		--user %s \
		%s \
		%s \
		%s \
		%s \
		php bin/console messenger:consume %s --time-limit=3600 --memory-limit=256M -vv`,
		workerName, constants.NetworkName, constants.ContainerUser, constants.DockerLogOptions, envVars, volumeMounts, imageName, transportsArg)

	result, err := client.Exec(ctx, workerCmd)
	if err != nil {
		return fmt.Errorf("failed to start worker: %w", err)
	}
	if err := result.Err(); err != nil {
		return fmt.Errorf("failed to start worker: %w", err)
	}

	return nil
}

// runEnvPreflightCheck verifies required environment variables before deployment
func runEnvPreflightCheck(ctx context.Context, client ssh.Executor, cfg *config.ProjectConfig, serverName string) error {
	result, err := deploy.CheckEnvVars(ctx, client, cfg, serverName)
	if err != nil {
		return fmt.Errorf("failed to check environment variables: %w", err)
	}

	// No missing variables, continue
	if len(result.Missing) == 0 {
		return nil
	}

	// Check if any variables can be auto-generated
	canGenerate := false
	for _, req := range result.Missing {
		if req.CanGenerate {
			canGenerate = true
			break
		}
	}

	// Interactive mode: prompt user
	if IsInteractive() && canGenerate {
		return handleInteractiveEnvCheck(ctx, client, cfg, result, serverName)
	}

	// Non-interactive mode or cannot generate: show error
	PrintError(deploy.FormatEnvCheckError(result.Missing, serverName))
	return fmt.Errorf("missing required environment variables")
}

// handleInteractiveEnvCheck handles missing env vars in interactive mode
func handleInteractiveEnvCheck(ctx context.Context, client ssh.Executor, cfg *config.ProjectConfig, result *deploy.EnvCheckResult, serverName string) error {
	// Show missing variables
	PrintWarning("Missing required environment variables:")
	for _, req := range result.Missing {
		fmt.Printf("   - %s\n", req.Name)
	}
	fmt.Println()

	// Check which variables cannot be auto-generated
	var nonGeneratable []deploy.EnvRequirement
	for _, req := range result.Missing {
		if !req.CanGenerate {
			nonGeneratable = append(nonGeneratable, req)
		}
	}

	// If there are non-generatable variables, we can't proceed automatically
	if len(nonGeneratable) > 0 {
		PrintError("The following variables cannot be auto-generated:")
		for _, req := range nonGeneratable {
			fmt.Printf("   - %s (%s)\n", req.Name, req.Description)
		}
		fmt.Println()
		PrintInfo("Run the following commands to configure them:")
		for _, req := range nonGeneratable {
			if req.Name == "DATABASE_URL" {
				fmt.Printf("   frankendeploy env set %s DATABASE_URL=\"postgresql://user:pass@host:5432/db\"\n", serverName)
			} else {
				fmt.Printf("   frankendeploy env set %s %s=\"<value>\"\n", serverName, req.Name)
			}
		}
		return fmt.Errorf("missing required environment variables that cannot be auto-generated")
	}

	// All missing variables can be generated - prompt user
	options := []string{
		"Generate missing secrets automatically (Recommended)",
		"Show commands to set manually",
	}

	choice := PromptSelect("How would you like to proceed?", options)

	switch choice {
	case 0: // Generate automatically
		generated, err := deploy.GenerateMissingSecrets(result.Missing)
		if err != nil {
			return fmt.Errorf("failed to generate secrets: %w", err)
		}

		if err := deploy.SaveGeneratedSecrets(ctx, client, cfg.Name, generated); err != nil {
			return fmt.Errorf("failed to save generated secrets: %w", err)
		}

		for key := range generated {
			PrintSuccess("Generated %s", key)
		}
		PrintInfo("Continuing deployment...")
		return nil

	case 1: // Show commands
		fmt.Println()
		PrintInfo("Run the following commands to configure them:")
		for _, req := range result.Missing {
			if req.Name == "APP_SECRET" {
				fmt.Printf("   frankendeploy env set %s APP_SECRET=$(openssl rand -hex 32)\n", serverName)
			} else {
				fmt.Printf("   frankendeploy env set %s %s=\"<value>\"\n", serverName, req.Name)
			}
		}
		return fmt.Errorf("deployment cancelled - please set environment variables first")

	default: // Skip (0)
		return fmt.Errorf("deployment cancelled by user")
	}
}

// checkAndWarnMigrationState checks for empty migrations and warns once per app
func checkAndWarnMigrationState(ctx context.Context, client ssh.Executor, appName string) {
	// Check migration state inside the container
	result, err := deploy.CheckMigrationState(ctx, client, appName)
	if err != nil {
		PrintVerbose("Could not check migration state: %v", err)
		return
	}

	// If migrations now exist, clear any previous warning marker
	if result.MigrationFilesCount > 0 {
		_ = deploy.ClearMigrationWarningMarker(ctx, client, appName)
		return
	}

	// No problem detected (no entities or migrations exist)
	if !result.HasPotentialProblem {
		return
	}

	// Check if we've already shown this warning for this app
	if deploy.HasMigrationWarningBeenShown(ctx, client, appName) {
		PrintVerbose("Migration warning already shown for this app")
		return
	}

	// Show the warning
	fmt.Println()
	PrintWarning(deploy.FormatMigrationWarning(result))

	// Mark warning as shown so we don't repeat it
	if err := deploy.MarkMigrationWarningShown(ctx, client, appName); err != nil {
		PrintVerbose("Could not mark migration warning as shown: %v", err)
	}
}

// checkArchitectureMismatch detects if local and server architectures are incompatible
// Returns: (shouldUseRemoteBuild bool, err error)
func checkArchitectureMismatch(ctx context.Context, client *ssh.Client, serverCfg *config.ServerConfig, globalCfg *config.GlobalConfig, serverName string) (bool, error) {
	// 1. Check explicit flags first
	if deployNoRemoteBuild {
		return false, nil // User explicitly wants local build
	}
	if deployRemoteBuild {
		return true, nil // User explicitly wants remote build
	}

	// 2. Check saved server preference
	if serverCfg.RemoteBuild != nil {
		if *serverCfg.RemoteBuild {
			PrintInfo("Using remote build (server configured for cross-architecture)")
		}
		return *serverCfg.RemoteBuild, nil
	}

	// 3. Detect architectures
	localArch := runtime.GOARCH // "arm64" on Mac Silicon, "amd64" on Intel
	serverArch, err := client.GetServerArchitecture(ctx)
	if err != nil {
		PrintWarning("Could not detect server architecture: %v", err)
		return false, nil // Default to local build
	}

	// Normalize architectures for comparison
	localNorm := normalizeArch(localArch)
	serverNorm := normalizeArch(serverArch)

	// 4. No mismatch - continue with local build
	if localNorm == serverNorm {
		return false, nil
	}

	// 5. Mismatch detected - handle based on mode
	return handleArchitectureMismatch(serverCfg, globalCfg, serverName, localArch, serverArch)
}

// normalizeArch converts architecture names to a common format
func normalizeArch(arch string) string {
	arch = strings.TrimSpace(strings.ToLower(arch))
	switch arch {
	case "arm64", "aarch64":
		return "arm64"
	case "amd64", "x86_64":
		return "amd64"
	default:
		return arch
	}
}

// handleArchitectureMismatch handles the case where local and server architectures differ
func handleArchitectureMismatch(serverCfg *config.ServerConfig, globalCfg *config.GlobalConfig, serverName, localArch, serverArch string) (bool, error) {
	// Display warning
	PrintWarning("Architecture mismatch detected:")
	fmt.Printf("   Local:  %s", localArch)
	if localArch == "arm64" {
		fmt.Printf(" (Apple Silicon)")
	}
	fmt.Println()
	fmt.Printf("   Server: %s\n", serverArch)
	fmt.Println()
	fmt.Println("   Local builds will not run on this server.")
	fmt.Println()

	// Non-interactive mode: fail with clear error
	if !IsInteractive() {
		PrintError("Architecture mismatch: local %s → server %s", localArch, serverArch)
		fmt.Println()
		fmt.Println("   Add --remote-build flag or configure server:")
		fmt.Printf("   frankendeploy server set %s remote_build true\n", serverName)
		return false, fmt.Errorf("architecture mismatch requires --remote-build flag in CI/CD mode")
	}

	// Interactive mode: prompt user
	options := []string{
		"Use remote build for this server (Recommended)",
		"Continue with local build anyway",
	}

	choice := PromptSelect("Use remote build for this server?", options)

	switch choice {
	case 0: // Use remote build
		// Save preference
		remoteBuild := true
		serverCfg.RemoteBuild = &remoteBuild
		globalCfg.Servers[serverName] = *serverCfg

		if err := config.SaveGlobalConfig(globalCfg); err != nil {
			PrintWarning("Could not save preference: %v", err)
		} else {
			PrintSuccess("Server '%s' configured for remote builds", serverName)
		}
		return true, nil

	case 1: // Local build
		PrintWarning("Continuing with local build (may fail with 'exec format error')")
		return false, nil

	default: // Skip/Cancel
		return false, fmt.Errorf("deployment cancelled by user")
	}
}
