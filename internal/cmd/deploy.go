package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/frankendeploy/internal/caddy"
	"github.com/yoanbernabeu/frankendeploy/internal/config"
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
	deployTag         string
	deployForce       bool
	deployNoBuild     bool
	deployRemoteBuild bool
)

func init() {
	rootCmd.AddCommand(deployCmd)
	deployCmd.Flags().StringVarP(&deployTag, "tag", "t", "", "Image tag (default: timestamp)")
	deployCmd.Flags().BoolVarP(&deployForce, "force", "f", false, "Force deployment even if checks fail")
	deployCmd.Flags().BoolVar(&deployNoBuild, "no-build", false, "Skip image build (use existing image)")
	deployCmd.Flags().BoolVar(&deployRemoteBuild, "remote-build", false, "Build image on the server (recommended for cross-architecture)")
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
	}

	imageName := fmt.Sprintf("%s:%s", projectCfg.Name, deployTag)
	remoteAppPath := filepath.Join("/opt/frankendeploy/apps", projectCfg.Name)

	PrintInfo("Deploying %s to %s...", projectCfg.Name, serverName)

	// Step 1: Connect to server
	PrintInfo("Connecting to %s...", serverCfg.Host)
	client := ssh.NewClient(serverCfg.Host, serverCfg.User, serverCfg.Port, serverCfg.KeyPath)
	if err := client.Connect(); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer client.Close()
	PrintSuccess("Connected")

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

	// Step 4: Deploy new version
	PrintInfo("Starting new version...")
	if err := deployNewVersion(client, projectCfg, imageName, remoteAppPath, deployTag, databaseURL); err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}

	// Step 4b: Deploy Messenger workers if enabled
	if projectCfg.Messenger.Enabled {
		PrintInfo("Starting Messenger workers...")
		if err := deployMessengerWorkers(client, projectCfg, imageName, remoteAppPath, databaseURL); err != nil {
			PrintWarning("Failed to start Messenger workers: %v", err)
		} else {
			PrintSuccess("Messenger workers started (%d workers)", projectCfg.Messenger.Workers)
		}
	}

	// Step 5: Run pre_deploy hooks
	if len(projectCfg.Deploy.Hooks.PreDeploy) > 0 {
		PrintInfo("Running pre-deploy hooks...")
		if err := runDeployHooks(client, projectCfg.Name, projectCfg.Deploy.Hooks.PreDeploy); err != nil {
			if !deployForce {
				PrintWarning("Pre-deploy hooks failed, rolling back...")
				_ = rollbackDeployment(client, projectCfg.Name, remoteAppPath)
				return fmt.Errorf("pre-deploy hooks failed: %w", err)
			}
			PrintWarning("Pre-deploy hooks failed but continuing (--force)")
		}
	}

	// Step 6: Health check
	PrintInfo("Running health check...")
	if err := runHealthCheck(client, projectCfg, remoteAppPath); err != nil {
		if !deployForce {
			// Rollback
			PrintWarning("Health check failed, rolling back...")
			_ = rollbackDeployment(client, projectCfg.Name, remoteAppPath)
			return fmt.Errorf("deployment failed health check: %w", err)
		}
		PrintWarning("Health check failed but continuing (--force)")
	}
	PrintSuccess("Health check passed")

	// Step 8: Run post_deploy hooks
	if len(projectCfg.Deploy.Hooks.PostDeploy) > 0 {
		PrintInfo("Running post-deploy hooks...")
		if err := runDeployHooks(client, projectCfg.Name, projectCfg.Deploy.Hooks.PostDeploy); err != nil {
			PrintWarning("Post-deploy hooks failed: %v", err)
			// Don't rollback for post-deploy hooks, app is already running
		}
	}

	// Step 9: Update Caddy config
	PrintInfo("Updating reverse proxy...")
	if err := updateCaddyConfig(client, projectCfg); err != nil {
		PrintWarning("Failed to update Caddy: %v", err)
	}

	// Step 7: Cleanup old releases
	PrintInfo("Cleaning up old releases...")
	cleanupOldReleases(client, remoteAppPath, projectCfg.Deploy.KeepReleases)

	PrintSuccess("Deployment complete!")
	fmt.Println()
	fmt.Printf("Application deployed: %s\n", projectCfg.Name)
	fmt.Printf("  Tag: %s\n", deployTag)
	if projectCfg.Deploy.Domain != "" {
		fmt.Printf("  URL: https://%s\n", projectCfg.Deploy.Domain)
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
	_, _ = client.Exec(fmt.Sprintf("rm -rf %s && mkdir -p %s", buildPath, buildPath))

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
	_, _ = client.Exec(fmt.Sprintf("rm -rf %s", buildPath))

	return nil
}

func deployNewVersion(client *ssh.Client, cfg *config.ProjectConfig, imageName, appPath, tag, databaseURL string) error {
	releasePath := filepath.Join(appPath, "releases", tag)
	currentPath := filepath.Join(appPath, "current")
	sharedPath := filepath.Join(appPath, "shared")

	// Prepare shared directories
	sharedDirs := cfg.Deploy.SharedDirs
	if len(sharedDirs) == 0 {
		// Default shared directories for Symfony
		sharedDirs = []string{"var/log", "var/sessions"}
	}

	// Prepare shared files
	sharedFiles := cfg.Deploy.SharedFiles
	if len(sharedFiles) == 0 {
		sharedFiles = []string{".env.local"}
	}

	// Build commands to create shared structure
	commands := []string{
		// Create release directory
		fmt.Sprintf("mkdir -p %s", releasePath),

		// Create shared base directory
		fmt.Sprintf("mkdir -p %s", sharedPath),
	}

	// Create shared directories
	for _, dir := range sharedDirs {
		commands = append(commands, fmt.Sprintf("mkdir -p %s/%s", sharedPath, dir))
	}

	// Create shared files (touch if not exists)
	for _, file := range sharedFiles {
		dirPath := filepath.Dir(file)
		if dirPath != "." {
			commands = append(commands, fmt.Sprintf("mkdir -p %s/%s", sharedPath, dirPath))
		}
		commands = append(commands, fmt.Sprintf("touch %s/%s", sharedPath, file))
	}

	// Stop old container if exists
	commands = append(commands,
		fmt.Sprintf("docker stop %s 2>/dev/null || true", cfg.Name),
		fmt.Sprintf("docker rm %s 2>/dev/null || true", cfg.Name),
	)

	// Build volume mounts
	volumeMounts := buildVolumeMounts(sharedPath, sharedDirs, sharedFiles)

	// Build environment variables
	envVars := "-e SERVER_NAME=:8080 -e APP_ENV=prod -e APP_DEBUG=0"
	if databaseURL != "" {
		envVars += fmt.Sprintf(" -e DATABASE_URL='%s'", databaseURL)
	}

	// Start new container with all shared volumes mounted
	// SECURITY: Run as non-root user (1000:1000) with non-privileged port 8080
	dockerRunCmd := fmt.Sprintf(`docker run -d --name %s \
		--network frankendeploy \
		--restart unless-stopped \
		--user 1000:1000 \
		%s \
		%s \
		%s`, cfg.Name, envVars, volumeMounts, imageName)

	commands = append(commands, dockerRunCmd)

	// Update current symlink and save release info
	commands = append(commands,
		fmt.Sprintf("ln -sfn %s %s", releasePath, currentPath),
		fmt.Sprintf("echo '%s' > %s/release", tag, releasePath),
	)

	for _, command := range commands {
		PrintVerbose("Running: %s", command)
		result, err := client.Exec(command)
		if err != nil {
			return fmt.Errorf("command failed: %w", err)
		}
		if result.ExitCode != 0 && !strings.Contains(command, "|| true") {
			return fmt.Errorf("command failed: %s", result.Stderr)
		}
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

func runHealthCheck(client *ssh.Client, cfg *config.ProjectConfig, appPath string) error {
	healthPath := cfg.Deploy.HealthcheckPath
	if healthPath == "" {
		healthPath = "/"
	}

	// Validate health path
	if err := security.ValidateHealthPath(healthPath); err != nil {
		return fmt.Errorf("invalid health check path: %w", err)
	}

	// Wait for container to be ready
	time.Sleep(5 * time.Second)

	// Check container is running
	result, err := client.Exec(fmt.Sprintf("docker ps --filter name=%s --format '{{.Status}}'", cfg.Name))
	if err != nil || result.Stdout == "" {
		return fmt.Errorf("container not running")
	}

	// Check health endpoint (port 8080 for non-root execution)
	healthCmd := fmt.Sprintf("docker exec %s curl -sf http://localhost:8080%s", cfg.Name, healthPath)
	result, err = client.Exec(healthCmd)
	if err != nil || result.ExitCode != 0 {
		return fmt.Errorf("health check failed")
	}

	return nil
}

func rollbackDeployment(client *ssh.Client, appName, appPath string) error {
	// Get previous release
	result, _ := client.Exec(fmt.Sprintf("ls -1t %s/releases | head -2 | tail -1", appPath))
	previousRelease := strings.TrimSpace(result.Stdout)

	if previousRelease == "" {
		return fmt.Errorf("no previous release to rollback to")
	}

	// Stop current containers (app + worker)
	_, _ = client.Exec(fmt.Sprintf("docker stop %s 2>/dev/null || true", appName))
	_, _ = client.Exec(fmt.Sprintf("docker rm %s 2>/dev/null || true", appName))
	_, _ = client.Exec(fmt.Sprintf("docker stop %s-worker 2>/dev/null || true", appName))
	_, _ = client.Exec(fmt.Sprintf("docker rm %s-worker 2>/dev/null || true", appName))

	// Restore previous release
	previousPath := filepath.Join(appPath, "releases", previousRelease)
	currentPath := filepath.Join(appPath, "current")
	_, _ = client.Exec(fmt.Sprintf("ln -sfn %s %s", previousPath, currentPath))

	return nil
}

func updateCaddyConfig(client *ssh.Client, cfg *config.ProjectConfig) error {
	domain := cfg.Deploy.Domain
	if domain == "" {
		PrintWarning("No domain configured in frankendeploy.yaml (deploy.domain)")
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
	commands := caddy.WriteAppConfigCommands(cfg.Name, configContent)
	for _, command := range commands {
		PrintVerbose("Running: %s", command)
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
		keepReleases = 5
	}

	// List releases and remove old ones
	cleanupCmd := fmt.Sprintf(
		"cd %s/releases && ls -1t | tail -n +%d | xargs -r rm -rf",
		appPath, keepReleases+1)

	_, _ = client.Exec(cleanupCmd)
}

// runDeployHooks executes deployment hooks inside the container
func runDeployHooks(client *ssh.Client, containerName string, hooks []string) error {
	for _, hook := range hooks {
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
		envVars += fmt.Sprintf(" -e DATABASE_URL='%s'", databaseURL)
	}

	// Stop existing workers
	stopCmd := fmt.Sprintf("docker stop %s 2>/dev/null || true && docker rm %s 2>/dev/null || true", workerName, workerName)
	_, _ = client.Exec(stopCmd)

	// Start worker container with messenger:consume command
	// SECURITY: Run as non-root user (1000:1000)
	workerCmd := fmt.Sprintf(`docker run -d --name %s \
		--network frankendeploy \
		--restart unless-stopped \
		--user 1000:1000 \
		%s \
		%s \
		%s \
		php bin/console messenger:consume %s --time-limit=3600 --memory-limit=256M -vv`,
		workerName, envVars, volumeMounts, imageName, transportsArg)

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
	result, _ := client.Exec(fmt.Sprintf("docker ps -q -f name=%s", dbContainerName))
	if strings.TrimSpace(result.Stdout) != "" {
		// Container exists, read existing credentials
		result, err := client.Exec(fmt.Sprintf("cat %s 2>/dev/null", credentialsFile))
		if err == nil && result.ExitCode == 0 && result.Stdout != "" {
			PrintVerbose("Using existing database container")
			return strings.TrimSpace(result.Stdout), nil
		}
	}

	// Generate credentials
	dbUser := cfg.Name
	dbPassword := generateRandomPassword(24)

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
		dockerEnv = fmt.Sprintf("-e POSTGRES_USER=%s -e POSTGRES_PASSWORD=%s -e POSTGRES_DB=%s", dbUser, dbPassword, dbName)
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
			dbPassword, dbUser, dbPassword, dbName)
		dockerPort = "3306"
		databaseURL = fmt.Sprintf("mysql://%s:%s@%s:%s/%s?serverVersion=%s&charset=utf8mb4",
			dbUser, dbPassword, dbContainerName, dockerPort, dbName, version)

	default:
		return "", fmt.Errorf("unsupported database driver for managed mode: %s", cfg.Database.Driver)
	}

	// Stop and remove existing container if exists (for recreation)
	_, _ = client.Exec(fmt.Sprintf("docker stop %s 2>/dev/null || true", dbContainerName))
	_, _ = client.Exec(fmt.Sprintf("docker rm %s 2>/dev/null || true", dbContainerName))

	// Create database container with persistent volume
	dbRunCmd := fmt.Sprintf(`docker run -d --name %s \
		--network frankendeploy \
		--restart unless-stopped \
		%s \
		-v %s-data:/var/lib/%s \
		%s`,
		dbContainerName,
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
	_, _ = client.Exec(fmt.Sprintf("echo '%s' > %s", databaseURL, credentialsFile))
	_, _ = client.Exec(fmt.Sprintf("chmod 600 %s", credentialsFile))

	// Wait for database to be ready
	PrintVerbose("Waiting for database to be ready...")
	for i := 0; i < 30; i++ {
		var checkCmd string
		if cfg.Database.Driver == "pgsql" {
			checkCmd = fmt.Sprintf("docker exec %s pg_isready -U %s", dbContainerName, dbUser)
		} else {
			checkCmd = fmt.Sprintf("docker exec %s mysqladmin ping -u%s -p%s --silent", dbContainerName, dbUser, dbPassword)
		}
		result, _ := client.Exec(checkCmd)
		if result.ExitCode == 0 {
			break
		}
		_, _ = client.Exec("sleep 1")
	}

	return databaseURL, nil
}

// generateRandomPassword generates a secure random password
func generateRandomPassword(length int) string {
	bytes := make([]byte, length/2)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
