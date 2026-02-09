package cmd

import (
	"crypto/rand"
	"encoding/hex"
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
	deployTag           string
	deployForce         bool
	deployNoBuild       bool
	deployRemoteBuild   bool
	deployNoRemoteBuild bool
)

func init() {
	rootCmd.AddCommand(deployCmd)
	deployCmd.Flags().StringVarP(&deployTag, "tag", "t", "", "Image tag (default: timestamp)")
	deployCmd.Flags().BoolVarP(&deployForce, "force", "f", false, "Force deployment even if checks fail")
	deployCmd.Flags().BoolVar(&deployNoBuild, "no-build", false, "Skip image build (use existing image)")
	deployCmd.Flags().BoolVar(&deployRemoteBuild, "remote-build", false, "Build image on the server (recommended for cross-architecture)")
	deployCmd.Flags().BoolVar(&deployNoRemoteBuild, "no-remote-build", false, "Force local build (ignore saved preference)")
}

func runDeploy(cmd *cobra.Command, args []string) error {
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

	// Validate server name
	if err := security.ValidateServerName(serverName); err != nil {
		return fmt.Errorf("invalid server name: %w", err)
	}

	// Load project config
	projectCfg, err := config.LoadProjectConfig(GetConfigFile())
	if err != nil {
		return err
	}

	// Load global config
	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	// Get server config
	serverCfg, err := globalCfg.GetServer(serverName)
	if err != nil {
		return err
	}

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

	PrintInfo("Deploying %s to %s...", projectCfg.Name, serverName)

	// Step 1: Connect to server
	PrintInfo("Connecting to %s...", serverCfg.Host)
	client := ssh.NewClient(serverCfg.Host, serverCfg.User, serverCfg.Port, serverCfg.KeyPath)
	if err := client.Connect(); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer client.Close()
	PrintSuccess("Connected")

	// Step 1a: Check architecture compatibility
	useRemoteBuild, err := checkArchitectureMismatch(client, serverCfg, globalCfg, serverName)
	if err != nil {
		return err
	}
	if useRemoteBuild && !deployRemoteBuild {
		deployRemoteBuild = true
	}

	// Step 1b: Pre-flight environment check
	if !deployForce {
		PrintInfo("Running pre-flight checks...")
		if err := runEnvPreflightCheck(client, projectCfg, serverName); err != nil {
			return err
		}
		PrintSuccess("Pre-flight checks passed")
	}

	if deployRemoteBuild {
		// Remote build: transfer source code and build on server
		PrintInfo("Transferring source code to server...")
		if err := transferSourceCode(client, serverCfg, projectCfg.Name, remoteAppPath); err != nil {
			return fmt.Errorf("transfer failed: %w", err)
		}
		PrintSuccess("Source code transferred")

		PrintInfo("Building Docker image on server...")
		if err := buildDockerImageRemote(client, imageName, remoteAppPath); err != nil {
			return fmt.Errorf("remote build failed: %w", err)
		}
		PrintSuccess("Image built: %s", imageName)
	} else {
		// Local build: build locally and transfer image
		if !deployNoBuild {
			PrintInfo("Building Docker image locally...")
			if err := buildDockerImage(imageName); err != nil {
				return fmt.Errorf("build failed: %w", err)
			}
			PrintSuccess("Image built: %s", imageName)
		}

		PrintInfo("Transferring image to server...")
		if err := transferImage(client, serverCfg, imageName); err != nil {
			return fmt.Errorf("transfer failed: %w", err)
		}
		PrintSuccess("Image transferred")
	}

	// Step 3b: Deploy managed database if configured
	var databaseURL string
	if projectCfg.Database.Driver != "" && projectCfg.Database.Managed != nil && *projectCfg.Database.Managed {
		PrintInfo("Setting up managed database...")
		var err error
		databaseURL, err = deployManagedDatabase(client, projectCfg, remoteAppPath)
		if err != nil {
			return fmt.Errorf("database setup failed: %w", err)
		}
		PrintSuccess("Database ready: %s", projectCfg.Database.Driver)
	}

	// Blue-green deployment: start new container with temp name, health check, then swap
	state := deploy.NewDeployState(projectCfg.Name)

	// Step 4: Prepare release directories and shared volumes
	PrintInfo("Preparing release...")
	state.Phase = deploy.PhasePrepareRelease
	if err := prepareRelease(client, projectCfg, remoteAppPath, deployTag); err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}

	// Check if old container exists (for swap phase)
	oldResult, _ := client.Exec(fmt.Sprintf("docker ps -q -f name=^%s$", projectCfg.Name))
	state.OldContainerExists = strings.TrimSpace(oldResult.Stdout) != ""

	// Step 5: Start new container with temporary name (old container still running)
	PrintInfo("Starting new version (blue-green)...")
	state.Phase = deploy.PhaseStartNewContainer
	if err := startNewContainer(client, projectCfg, imageName, remoteAppPath, deployTag, databaseURL, state.TempContainerName); err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}

	// Step 6: Run pre-deploy hooks on the NEW container
	if len(projectCfg.Deploy.Hooks.PreDeploy) > 0 {
		PrintInfo("Running pre-deploy hooks...")
		state.Phase = deploy.PhasePreDeployHooks
		if err := runDeployHooks(client, state.TempContainerName, projectCfg.Deploy.Hooks.PreDeploy); err != nil {
			if !deployForce {
				PrintWarning("Pre-deploy hooks failed, rolling back...")
				rollbackNewContainer(client, state)
				return fmt.Errorf("pre-deploy hooks failed: %w", err)
			}
			PrintWarning("Pre-deploy hooks failed but continuing (--force)")
		}

		// Check for empty migrations if migration hook was run
		if deploy.HasMigrationHook(projectCfg.Deploy.Hooks.PreDeploy) {
			checkAndWarnMigrationState(client, state.TempContainerName)
		}
	}

	// Step 7: Health check on the NEW container (old still running = zero downtime)
	PrintInfo("Running health check...")
	state.Phase = deploy.PhaseHealthCheck
	if err := runHealthCheckOnContainer(client, projectCfg, state.TempContainerName); err != nil {
		if !deployForce {
			PrintWarning("Health check failed, rolling back...")
			rollbackNewContainer(client, state)
			return fmt.Errorf("deployment failed health check: %w", err)
		}
		PrintWarning("Health check failed but continuing (--force)")
	}
	PrintSuccess("Health check passed")

	// Step 8: Swap containers (stop old, rename new → final, update symlink)
	PrintInfo("Swapping containers...")
	state.Phase = deploy.PhaseSwapContainers
	if err := swapContainers(client, projectCfg.Name, remoteAppPath, deployTag, state.TempContainerName); err != nil {
		return fmt.Errorf("swap failed: %w", err)
	}

	// Step 8b: Deploy Messenger workers if enabled
	if projectCfg.Messenger.Enabled {
		PrintInfo("Starting Messenger workers...")
		if err := deployMessengerWorkers(client, projectCfg, imageName, remoteAppPath, databaseURL); err != nil {
			PrintWarning("Failed to start Messenger workers: %v", err)
		} else {
			PrintSuccess("Messenger workers started (%d workers)", projectCfg.Messenger.Workers)
		}
	}

	// Step 9: Run post_deploy hooks
	state.Phase = deploy.PhasePostDeployHooks
	if len(projectCfg.Deploy.Hooks.PostDeploy) > 0 {
		PrintInfo("Running post-deploy hooks...")
		if err := runDeployHooks(client, projectCfg.Name, projectCfg.Deploy.Hooks.PostDeploy); err != nil {
			PrintWarning("Post-deploy hooks failed: %v", err)
		}
	}

	// Step 10: Update Caddy config
	PrintInfo("Updating reverse proxy...")
	if err := updateCaddyConfig(client, projectCfg); err != nil {
		PrintWarning("Failed to update Caddy: %v", err)
	}

	// Step 11: Cleanup old releases
	state.Phase = deploy.PhaseCleanup
	PrintInfo("Cleaning up old releases...")
	cleanupOldReleases(client, remoteAppPath, projectCfg.Deploy.KeepReleases)
	state.Phase = deploy.PhaseDone

	PrintSuccess("Deployment complete!")
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

func buildDockerImage(imageName string) error {
	// Use buildx to cross-compile for linux/amd64 (VPS architecture)
	dockerCmd := exec.Command("docker", "buildx", "build",
		"--platform", "linux/amd64",
		"--target", "frankenphp_prod",
		"--load",
		"-t", imageName,
		".")
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr
	return dockerCmd.Run()
}

func transferImage(client *ssh.Client, serverCfg *config.ServerConfig, imageName string) error {
	// Save image to tar
	tarPath := fmt.Sprintf("/tmp/%s.tar", strings.ReplaceAll(imageName, ":", "-"))

	saveCmd := exec.Command("docker", "save", "-o", tarPath, imageName)
	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}
	defer os.Remove(tarPath)

	// Get image size for progress
	info, _ := os.Stat(tarPath)
	PrintVerbose("Image size: %.2f MB", float64(info.Size())/1024/1024)

	// Upload using scp
	remoteTarPath := fmt.Sprintf("/tmp/%s.tar", strings.ReplaceAll(imageName, ":", "-"))

	scpArgs := []string{
		"-P", fmt.Sprintf("%d", serverCfg.Port),
		"-o", "StrictHostKeyChecking=no",
	}
	if serverCfg.KeyPath != "" {
		scpArgs = append(scpArgs, "-i", serverCfg.KeyPath)
	}
	scpArgs = append(scpArgs, tarPath, fmt.Sprintf("%s@%s:%s", serverCfg.User, serverCfg.Host, remoteTarPath))

	scpCmd := exec.Command("scp", scpArgs...)
	if err := scpCmd.Run(); err != nil {
		return fmt.Errorf("failed to upload image: %w", err)
	}

	// Load image on remote
	result, err := client.Exec(fmt.Sprintf("docker load -i %s && rm %s", remoteTarPath, remoteTarPath))
	if err != nil || result.ExitCode != 0 {
		return fmt.Errorf("failed to load image on server: %s", result.Stderr)
	}

	return nil
}

func transferSourceCode(client *ssh.Client, serverCfg *config.ServerConfig, appName, appPath string) error {
	// Create build directory on server
	buildPath := fmt.Sprintf("%s/build", appPath)
	if _, err := client.Exec(fmt.Sprintf("rm -rf %s && mkdir -p %s", buildPath, buildPath)); err != nil {
		return fmt.Errorf("failed to create build directory: %w", err)
	}

	// Use rsync to transfer source code (respects .dockerignore patterns)
	rsyncArgs := []string{
		"-avz", "--delete",
		"--exclude", ".git",
		"--exclude", "node_modules",
		"--exclude", "vendor",
		"--exclude", "var",
		"--exclude", ".env.local",
		"-e", fmt.Sprintf("ssh -p %d -o StrictHostKeyChecking=no -i %s", serverCfg.Port, serverCfg.KeyPath),
		"./",
		fmt.Sprintf("%s@%s:%s/", serverCfg.User, serverCfg.Host, buildPath),
	}

	// If no key path, remove the -i option
	if serverCfg.KeyPath == "" {
		rsyncArgs = []string{
			"-avz", "--delete",
			"--exclude", ".git",
			"--exclude", "node_modules",
			"--exclude", "vendor",
			"--exclude", "var",
			"--exclude", ".env.local",
			"-e", fmt.Sprintf("ssh -p %d -o StrictHostKeyChecking=no", serverCfg.Port),
			"./",
			fmt.Sprintf("%s@%s:%s/", serverCfg.User, serverCfg.Host, buildPath),
		}
	}

	rsyncCmd := exec.Command("rsync", rsyncArgs...)
	rsyncCmd.Stdout = os.Stdout
	rsyncCmd.Stderr = os.Stderr
	if err := rsyncCmd.Run(); err != nil {
		return fmt.Errorf("rsync failed: %w", err)
	}

	return nil
}

func buildDockerImageRemote(client *ssh.Client, imageName, appPath string) error {
	buildPath := fmt.Sprintf("%s/build", appPath)

	// Build Docker image on the server
	buildCmd := fmt.Sprintf("cd %s && docker build --target frankenphp_prod -t %s .", buildPath, imageName)

	result, err := client.Exec(buildCmd)
	if err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("docker build failed: %s", result.Stderr)
	}

	// Cleanup build directory
	if _, err := client.Exec(fmt.Sprintf("rm -rf %s", buildPath)); err != nil {
		PrintVerbose("Could not cleanup build directory: %v", err)
	}

	return nil
}

// prepareRelease creates the release directory, shared directories and files, and fixes permissions.
func prepareRelease(client *ssh.Client, cfg *config.ProjectConfig, appPath, tag string) error {
	releasePath := filepath.Join(appPath, "releases", tag)
	sharedPath := filepath.Join(appPath, "shared")

	sharedDirs := cfg.Deploy.SharedDirs
	if len(sharedDirs) == 0 {
		sharedDirs = []string{"var/log", "var/sessions"}
	}

	sharedFiles := cfg.Deploy.SharedFiles
	if len(sharedFiles) == 0 {
		sharedFiles = []string{".env.local"}
	}

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
		result, err := client.Exec(command)
		if err != nil {
			return fmt.Errorf("command failed: %w", err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("command failed: %s", result.Stderr)
		}
	}

	fixSharedPermissions(client, sharedPath, sharedDirs, sharedFiles)
	return nil
}

// startNewContainer starts the new version with a temporary container name.
// The old container remains running for zero-downtime deployment.
func startNewContainer(client *ssh.Client, cfg *config.ProjectConfig, imageName, appPath, tag, databaseURL, containerName string) error {
	sharedPath := filepath.Join(appPath, "shared")

	sharedDirs := cfg.Deploy.SharedDirs
	if len(sharedDirs) == 0 {
		sharedDirs = []string{"var/log", "var/sessions"}
	}
	sharedFiles := cfg.Deploy.SharedFiles
	if len(sharedFiles) == 0 {
		sharedFiles = []string{".env.local"}
	}

	volumeMounts := buildVolumeMounts(sharedPath, sharedDirs, sharedFiles)

	envVars := fmt.Sprintf("-e SERVER_NAME=:%s -e APP_ENV=prod -e APP_DEBUG=0", constants.AppPort)
	if databaseURL != "" {
		envVars += fmt.Sprintf(" -e DATABASE_URL=%s", security.ShellEscape(databaseURL))
	}

	// Remove any leftover temp container from a previous failed deploy
	if _, err := client.Exec(fmt.Sprintf("docker rm -f %s 2>/dev/null || true", containerName)); err != nil {
		PrintVerbose("Could not cleanup old temp container: %v", err)
	}

	// SECURITY: Run as non-root user with non-privileged port
	dockerRunCmd := fmt.Sprintf(`docker run -d --name %s \
		--network %s \
		--restart unless-stopped \
		--user %s \
		%s \
		%s \
		%s`, containerName, constants.NetworkName, constants.ContainerUser, envVars, volumeMounts, imageName)

	PrintVerboseCommand(dockerRunCmd)
	result, err := client.Exec(dockerRunCmd)
	if err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to start container: %s", result.Stderr)
	}

	return nil
}

// swapContainers performs the atomic swap: stop old container, rename new → final, update symlink.
func swapContainers(client *ssh.Client, appName, appPath, tag, tempContainerName string) error {
	releasePath := filepath.Join(appPath, "releases", tag)
	currentPath := filepath.Join(appPath, "current")

	// Stop and remove old container
	if _, err := client.Exec(fmt.Sprintf("docker stop %s 2>/dev/null || true", appName)); err != nil {
		PrintVerbose("Could not stop old container: %v", err)
	}
	if _, err := client.Exec(fmt.Sprintf("docker rm %s 2>/dev/null || true", appName)); err != nil {
		PrintVerbose("Could not remove old container: %v", err)
	}

	// Rename temp container to final name
	result, err := client.Exec(fmt.Sprintf("docker rename %s %s", tempContainerName, appName))
	if err != nil {
		return fmt.Errorf("failed to rename container: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to rename container: %s", result.Stderr)
	}

	// Update current symlink and save release info
	if _, err := client.Exec(fmt.Sprintf("ln -sfn %s %s", releasePath, currentPath)); err != nil {
		return fmt.Errorf("failed to update symlink: %w", err)
	}
	if _, err := client.Exec(fmt.Sprintf("echo '%s' > %s/release", tag, releasePath)); err != nil {
		PrintVerbose("Could not write release file: %v", err)
	}

	return nil
}

// rollbackNewContainer removes the temporary new container, leaving the old one intact.
func rollbackNewContainer(client *ssh.Client, state *deploy.DeployState) {
	actions := state.RollbackActions()
	for _, action := range actions {
		PrintVerboseCommand(action)
		if _, err := client.Exec(action); err != nil {
			PrintVerbose("Rollback action failed: %v", err)
		}
	}
}

// runHealthCheckOnContainer runs a health check against a specific container name.
func runHealthCheckOnContainer(client *ssh.Client, cfg *config.ProjectConfig, containerName string) error {
	healthPath := cfg.Deploy.HealthcheckPath
	if healthPath == "" {
		healthPath = "/"
	}

	if err := security.ValidateHealthPath(healthPath); err != nil {
		return fmt.Errorf("invalid health check path: %w", err)
	}

	// Wait for container to be ready
	time.Sleep(5 * time.Second)

	// Check container is running
	result, err := client.Exec(fmt.Sprintf("docker ps --filter name=%s --format '{{.Status}}'", containerName))
	if err != nil || result.Stdout == "" {
		return fmt.Errorf("container %s not running", containerName)
	}

	// Check health endpoint (port 8080 for non-root execution)
	healthCmd := fmt.Sprintf("docker exec %s curl -sf http://localhost:%s%s", containerName, constants.AppPort, healthPath)
	result, err = client.Exec(healthCmd)
	if err != nil || result.ExitCode != 0 {
		return fmt.Errorf("health check failed on %s", containerName)
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
func fixSharedPermissions(client *ssh.Client, sharedPath string, sharedDirs, sharedFiles []string) {
	// Fix ownership of shared directory itself
	cmd := fmt.Sprintf("sudo chown %s %s 2>/dev/null || true", constants.ContainerUser, sharedPath)
	PrintVerboseCommand(cmd)
	if _, err := client.Exec(cmd); err != nil {
		PrintWarning("Could not fix permissions for shared path: %v", err)
	}

	// Fix ownership of shared directories (recursively for contents)
	for _, dir := range sharedDirs {
		dirPath := fmt.Sprintf("%s/%s", sharedPath, dir)
		cmd := fmt.Sprintf("sudo chown -R %s %s 2>/dev/null || true", constants.ContainerUser, dirPath)
		PrintVerboseCommand(cmd)
		result, err := client.Exec(cmd)
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
		if _, err := client.Exec(cmd); err != nil {
			PrintWarning("Could not fix ownership for %s: %v", file, err)
		}

		// Set restrictive permissions for .env files (contains secrets)
		if strings.HasPrefix(file, ".env") || strings.Contains(file, "/.env") {
			cmd = fmt.Sprintf("sudo chmod 600 %s 2>/dev/null || true", filePath)
			PrintVerboseCommand(cmd)
			if _, err := client.Exec(cmd); err != nil {
				PrintWarning("Could not set permissions for %s: %v", file, err)
			}
		}
	}
}


func updateCaddyConfig(client *ssh.Client, cfg *config.ProjectConfig) error {
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
		result, err := client.Exec(command)
		if err != nil {
			return fmt.Errorf("command failed: %w", err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("command failed: %s", result.Stderr)
		}
	}

	PrintSuccess("Caddy configured for %s", domain)
	return nil
}

func cleanupOldReleases(client *ssh.Client, appPath string, keepReleases int) {
	if keepReleases <= 0 {
		keepReleases = constants.DefaultKeepReleases
	}

	// List releases and remove old ones
	cleanupCmd := fmt.Sprintf(
		"cd %s/releases && ls -1t | tail -n +%d | xargs -r rm -rf",
		appPath, keepReleases+1)

	if _, err := client.Exec(cleanupCmd); err != nil {
		PrintVerbose("Could not cleanup old releases: %v", err)
	}
}

// runDeployHooks executes deployment hooks inside the container
func runDeployHooks(client *ssh.Client, containerName string, hooks []string) error {
	for _, hook := range hooks {
		// Validate hook command before execution
		if err := security.ValidateDockerCommand(hook); err != nil {
			return fmt.Errorf("invalid hook command %q: %w", hook, err)
		}
		PrintVerbose("  > %s", hook)
		// Execute hook inside the container
		cmd := fmt.Sprintf("docker exec %s %s", containerName, hook)
		result, err := client.Exec(cmd)
		if err != nil {
			return fmt.Errorf("hook failed: %w", err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("hook '%s' failed (exit %d): %s", hook, result.ExitCode, result.Stderr)
		}
	}
	return nil
}

// deployMessengerWorkers starts Messenger worker containers
func deployMessengerWorkers(client *ssh.Client, cfg *config.ProjectConfig, imageName, appPath, databaseURL string) error {
	workerName := fmt.Sprintf("%s-worker", cfg.Name)
	workers := cfg.Messenger.Workers
	if workers <= 0 {
		workers = 2
	}

	// Build transports argument
	transports := cfg.Messenger.Transports
	if len(transports) == 0 {
		transports = []string{"async"}
	}
	transportsArg := strings.Join(transports, " ")

	// Get shared dirs and files (same as main container)
	sharedPath := filepath.Join(appPath, "shared")
	sharedDirs := cfg.Deploy.SharedDirs
	if len(sharedDirs) == 0 {
		sharedDirs = []string{"var/log", "var/sessions"}
	}
	sharedFiles := cfg.Deploy.SharedFiles
	if len(sharedFiles) == 0 {
		sharedFiles = []string{".env.local"}
	}

	// Build volume mounts (same as main container)
	volumeMounts := buildVolumeMounts(sharedPath, sharedDirs, sharedFiles)

	// Build environment variables
	envVars := "-e APP_ENV=prod -e APP_DEBUG=0"
	if databaseURL != "" {
		envVars += fmt.Sprintf(" -e DATABASE_URL=%s", security.ShellEscape(databaseURL))
	}

	// Stop existing workers
	stopCmd := fmt.Sprintf("docker stop %s 2>/dev/null || true && docker rm %s 2>/dev/null || true", workerName, workerName)
	if _, err := client.Exec(stopCmd); err != nil {
		PrintVerbose("Could not stop existing worker: %v", err)
	}

	// Start worker container with messenger:consume command
	// SECURITY: Run as non-root user
	workerCmd := fmt.Sprintf(`docker run -d --name %s \
		--network %s \
		--restart unless-stopped \
		--user %s \
		%s \
		%s \
		%s \
		php bin/console messenger:consume %s --time-limit=3600 --memory-limit=256M -vv`,
		workerName, constants.NetworkName, constants.ContainerUser, envVars, volumeMounts, imageName, transportsArg)

	result, err := client.Exec(workerCmd)
	if err != nil {
		return fmt.Errorf("failed to start worker: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to start worker: %s", result.Stderr)
	}

	// If more than 1 worker requested, use docker exec to spawn additional workers
	// Note: For true scaling, consider using docker-compose or swarm
	if workers > 1 {
		PrintVerbose("Note: Running %d workers via single container (use docker-compose for true scaling)", workers)
	}

	return nil
}

// deployManagedDatabase creates and manages a database container for the app
func deployManagedDatabase(client *ssh.Client, cfg *config.ProjectConfig, appPath string) (string, error) {
	dbContainerName := fmt.Sprintf("%s-db", cfg.Name)
	dbName := strings.ReplaceAll(cfg.Name, "-", "_")
	credentialsFile := filepath.Join(appPath, "shared", ".db_credentials")

	// Check if database container already exists and is running
	checkResult, checkErr := client.Exec(fmt.Sprintf("docker ps -q -f name=%s", dbContainerName))
	if checkErr == nil && strings.TrimSpace(checkResult.Stdout) != "" {
		// Container exists, read existing credentials
		credResult, credErr := client.Exec(fmt.Sprintf("cat %s 2>/dev/null", credentialsFile))
		if credErr == nil && credResult.ExitCode == 0 && credResult.Stdout != "" {
			PrintVerbose("Using existing database container")
			return strings.TrimSpace(credResult.Stdout), nil
		}
	}

	// Generate credentials
	dbUser := cfg.Name
	dbPassword, err := generateRandomPassword(24)
	if err != nil {
		return "", err
	}

	// Build DATABASE_URL based on driver
	var databaseURL string
	var dockerImage string
	var dockerEnv string
	var dockerPort string

	switch cfg.Database.Driver {
	case "pgsql":
		version := cfg.Database.Version
		if version == "" {
			version = "16"
		}
		dockerImage = fmt.Sprintf("postgres:%s-alpine", version)
		dockerEnv = fmt.Sprintf("-e POSTGRES_USER=%s -e POSTGRES_PASSWORD=%s -e POSTGRES_DB=%s",
			security.ShellEscape(dbUser), security.ShellEscape(dbPassword), security.ShellEscape(dbName))
		dockerPort = "5432"
		databaseURL = fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?serverVersion=%s&charset=utf8",
			dbUser, dbPassword, dbContainerName, dockerPort, dbName, version)

	case "mysql":
		version := cfg.Database.Version
		if version == "" {
			version = "8.0"
		}
		dockerImage = fmt.Sprintf("mysql:%s", version)
		dockerEnv = fmt.Sprintf("-e MYSQL_ROOT_PASSWORD=%s -e MYSQL_USER=%s -e MYSQL_PASSWORD=%s -e MYSQL_DATABASE=%s",
			security.ShellEscape(dbPassword), security.ShellEscape(dbUser), security.ShellEscape(dbPassword), security.ShellEscape(dbName))
		dockerPort = "3306"
		databaseURL = fmt.Sprintf("mysql://%s:%s@%s:%s/%s?serverVersion=%s&charset=utf8mb4",
			dbUser, dbPassword, dbContainerName, dockerPort, dbName, version)

	default:
		return "", fmt.Errorf("unsupported database driver for managed mode: %s", cfg.Database.Driver)
	}

	// Stop and remove existing container if exists (for recreation)
	if _, err := client.Exec(fmt.Sprintf("docker stop %s 2>/dev/null || true", dbContainerName)); err != nil {
		PrintVerbose("Could not stop existing db container: %v", err)
	}
	if _, err := client.Exec(fmt.Sprintf("docker rm %s 2>/dev/null || true", dbContainerName)); err != nil {
		PrintVerbose("Could not remove existing db container: %v", err)
	}

	// Create database container with persistent volume
	dbRunCmd := fmt.Sprintf(`docker run -d --name %s \
		--network %s \
		--restart unless-stopped \
		%s \
		-v %s-data:/var/lib/%s \
		%s`,
		dbContainerName,
		constants.NetworkName,
		dockerEnv,
		dbContainerName,
		map[string]string{"pgsql": "postgresql/data", "mysql": "mysql"}[cfg.Database.Driver],
		dockerImage)

	result, err := client.Exec(dbRunCmd)
	if err != nil {
		return "", fmt.Errorf("failed to start database container: %w", err)
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("failed to start database container: %s", result.Stderr)
	}

	// Save credentials to file
	if _, err := client.Exec(fmt.Sprintf("echo %s > %s", security.ShellEscape(databaseURL), credentialsFile)); err != nil {
		return "", fmt.Errorf("failed to save database credentials: %w", err)
	}
	if _, err := client.Exec(fmt.Sprintf("chmod 600 %s", credentialsFile)); err != nil {
		PrintWarning("Could not set permissions on credentials file: %v", err)
	}

	// Wait for database to be ready
	PrintVerbose("Waiting for database to be ready...")
	for i := 0; i < 30; i++ {
		var checkCmd string
		if cfg.Database.Driver == "pgsql" {
			checkCmd = fmt.Sprintf("docker exec %s pg_isready -U %s", dbContainerName, security.ShellEscape(dbUser))
		} else {
			checkCmd = fmt.Sprintf("docker exec %s mysqladmin ping -u%s -p%s --silent",
				dbContainerName, security.ShellEscape(dbUser), security.ShellEscape(dbPassword))
		}
		checkResult, _ := client.Exec(checkCmd)
		if checkResult != nil && checkResult.ExitCode == 0 {
			break
		}
		time.Sleep(1 * time.Second)
	}

	return databaseURL, nil
}

// generateRandomPassword generates a secure random password
func generateRandomPassword(length int) (string, error) {
	b := make([]byte, length/2)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random password: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// runEnvPreflightCheck verifies required environment variables before deployment
func runEnvPreflightCheck(client *ssh.Client, cfg *config.ProjectConfig, serverName string) error {
	result, err := deploy.CheckEnvVars(client, cfg, serverName)
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
		return handleInteractiveEnvCheck(client, cfg, result, serverName)
	}

	// Non-interactive mode or cannot generate: show error
	PrintError(deploy.FormatEnvCheckError(result.Missing, serverName))
	return fmt.Errorf("missing required environment variables")
}

// handleInteractiveEnvCheck handles missing env vars in interactive mode
func handleInteractiveEnvCheck(client *ssh.Client, cfg *config.ProjectConfig, result *deploy.EnvCheckResult, serverName string) error {
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

		if err := deploy.SaveGeneratedSecrets(client, cfg.Name, generated); err != nil {
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
func checkAndWarnMigrationState(client *ssh.Client, appName string) {
	// Check migration state inside the container
	result, err := deploy.CheckMigrationState(client, appName)
	if err != nil {
		PrintVerbose("Could not check migration state: %v", err)
		return
	}

	// If migrations now exist, clear any previous warning marker
	if result.MigrationFilesCount > 0 {
		_ = deploy.ClearMigrationWarningMarker(client, appName)
		return
	}

	// No problem detected (no entities or migrations exist)
	if !result.HasPotentialProblem {
		return
	}

	// Check if we've already shown this warning for this app
	if deploy.HasMigrationWarningBeenShown(client, appName) {
		PrintVerbose("Migration warning already shown for this app")
		return
	}

	// Show the warning
	fmt.Println()
	PrintWarning(deploy.FormatMigrationWarning(result))

	// Mark warning as shown so we don't repeat it
	if err := deploy.MarkMigrationWarningShown(client, appName); err != nil {
		PrintVerbose("Could not mark migration warning as shown: %v", err)
	}
}

// checkArchitectureMismatch detects if local and server architectures are incompatible
// Returns: (shouldUseRemoteBuild bool, err error)
func checkArchitectureMismatch(client *ssh.Client, serverCfg *config.ServerConfig, globalCfg *config.GlobalConfig, serverName string) (bool, error) {
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
	serverArch, err := client.GetServerArchitecture()
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
