package deploy

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

// Orchestrator handles deployment workflow
type Orchestrator struct {
	client    *ssh.Client
	config    *config.ProjectConfig
	server    *config.ServerConfig
	tag       string
	basePath  string
	verbose   bool
	onMessage func(string)
}

// NewOrchestrator creates a new deployment orchestrator
func NewOrchestrator(client *ssh.Client, cfg *config.ProjectConfig, server *config.ServerConfig) (*Orchestrator, error) {
	if err := security.ValidateAppName(cfg.Name); err != nil {
		return nil, fmt.Errorf("invalid app name: %w", err)
	}
	return &Orchestrator{
		client:   client,
		config:   cfg,
		server:   server,
		basePath: filepath.Join("/opt/frankendeploy/apps", cfg.Name),
	}, nil
}

// SetTag sets the deployment tag
func (o *Orchestrator) SetTag(tag string) {
	o.tag = tag
}

// SetVerbose enables verbose output
func (o *Orchestrator) SetVerbose(verbose bool) {
	o.verbose = verbose
}

// OnMessage sets a callback for status messages
func (o *Orchestrator) OnMessage(fn func(string)) {
	o.onMessage = fn
}

func (o *Orchestrator) message(msg string) {
	if o.onMessage != nil {
		o.onMessage(msg)
	}
}

// Deploy performs the full deployment workflow
func (o *Orchestrator) Deploy(imageName string) error {
	if o.tag == "" {
		o.tag = time.Now().Format("20060102-150405")
	}

	releasePath := filepath.Join(o.basePath, "releases", o.tag)
	currentPath := filepath.Join(o.basePath, "current")
	sharedPath := filepath.Join(o.basePath, "shared")

	// Step 1: Prepare directories
	o.message("Preparing directories...")
	if err := o.prepareDirectories(releasePath, sharedPath); err != nil {
		return fmt.Errorf("failed to prepare directories: %w", err)
	}

	// Step 2: Run pre-deploy hooks
	if len(o.config.Deploy.Hooks.PreDeploy) > 0 {
		o.message("Running pre-deploy hooks...")
		if err := o.runHooks(o.config.Deploy.Hooks.PreDeploy); err != nil {
			return fmt.Errorf("pre-deploy hook failed: %w", err)
		}
	}

	// Step 3: Stop old container
	o.message("Stopping old container...")
	o.stopContainer()

	// Step 4: Start new container
	o.message("Starting new container...")
	if err := o.startContainer(imageName); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Step 5: Health check
	o.message("Running health check...")
	if err := o.healthCheck(); err != nil {
		o.message("Health check failed, rolling back...")
		o.stopContainer()
		return fmt.Errorf("health check failed: %w", err)
	}

	// Step 6: Update current symlink
	o.message("Activating new release...")
	if err := o.activateRelease(releasePath, currentPath); err != nil {
		return fmt.Errorf("failed to activate release: %w", err)
	}

	// Step 7: Run post-deploy hooks
	if len(o.config.Deploy.Hooks.PostDeploy) > 0 {
		o.message("Running post-deploy hooks...")
		if err := o.runHooks(o.config.Deploy.Hooks.PostDeploy); err != nil {
			return fmt.Errorf("post-deploy hook failed: %w", err)
		}
	}

	// Step 8: Cleanup old releases
	o.message("Cleaning up old releases...")
	o.cleanupReleases()

	return nil
}

func (o *Orchestrator) prepareDirectories(releasePath, sharedPath string) error {
	commands := []string{
		fmt.Sprintf("mkdir -p %s", releasePath),
		fmt.Sprintf("mkdir -p %s", sharedPath),
		// Ensure .env.local exists for environment variables
		fmt.Sprintf("touch %s/.env.local", sharedPath),
	}

	// Create shared directories
	for _, dir := range o.config.Deploy.SharedDirs {
		if err := security.ValidateSharedDir(dir); err != nil {
			return fmt.Errorf("invalid shared directory %q: %w", dir, err)
		}
		commands = append(commands, fmt.Sprintf("mkdir -p %s/%s", sharedPath, dir))
	}

	if err := o.runCommands(commands); err != nil {
		return err
	}

	// Fix permissions for container user 1000:1000
	o.fixSharedPermissions(sharedPath)

	return nil
}

// fixSharedPermissions ensures shared directories have correct ownership for container user 1000:1000
func (o *Orchestrator) fixSharedPermissions(sharedPath string) {
	// Fix shared directory ownership
	cmd := fmt.Sprintf("sudo chown 1000:1000 %s 2>/dev/null || true", sharedPath)
	_, _ = o.client.Exec(cmd)

	// Fix shared subdirectories recursively
	for _, dir := range o.config.Deploy.SharedDirs {
		dirPath := fmt.Sprintf("%s/%s", sharedPath, dir)
		cmd := fmt.Sprintf("sudo chown -R 1000:1000 %s 2>/dev/null || true", dirPath)
		_, _ = o.client.Exec(cmd)
	}

	// Fix .env.local ownership and permissions
	envPath := fmt.Sprintf("%s/.env.local", sharedPath)
	_, _ = o.client.Exec(fmt.Sprintf("sudo chown 1000:1000 %s 2>/dev/null || true", envPath))
	_, _ = o.client.Exec(fmt.Sprintf("sudo chmod 600 %s 2>/dev/null || true", envPath))
}

func (o *Orchestrator) stopContainer() {
	_, _ = o.client.Exec(fmt.Sprintf("docker stop %s 2>/dev/null || true", o.config.Name))
	_, _ = o.client.Exec(fmt.Sprintf("docker rm %s 2>/dev/null || true", o.config.Name))
}

func (o *Orchestrator) startContainer(imageName string) error {
	// Build environment variables
	// SECURITY: Use non-privileged port 8080 for non-root execution
	envVars := []string{
		"-e SERVER_NAME=:8080",
		"-e APP_ENV=prod",
		"-e APP_DEBUG=0",
	}

	for key, value := range o.config.Env.Prod {
		envVars = append(envVars, fmt.Sprintf("-e %s=%s", key, security.ShellEscape(value)))
	}

	// Mount shared .env.local for runtime environment variables
	envLocalPath := filepath.Join(o.basePath, "shared", ".env.local")

	// SECURITY: Run as non-root user (1000:1000)
	cmd := fmt.Sprintf(`docker run -d --name %s \
		--network frankendeploy \
		--restart unless-stopped \
		--user 1000:1000 \
		-v %s:/app/.env.local:ro \
		%s \
		%s`,
		o.config.Name,
		envLocalPath,
		strings.Join(envVars, " "),
		imageName)

	result, err := o.client.Exec(cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("docker run failed: %s", result.Stderr)
	}

	return nil
}

func (o *Orchestrator) healthCheck() error {
	healthPath := o.config.Deploy.HealthcheckPath
	if healthPath == "" {
		healthPath = "/"
	}

	// Wait for container to be ready
	time.Sleep(5 * time.Second)

	// Check container is running
	result, err := o.client.Exec(fmt.Sprintf("docker ps --filter name=%s --format '{{.Status}}'", o.config.Name))
	if err != nil || result.Stdout == "" {
		return fmt.Errorf("container not running")
	}

	// Check health endpoint
	healthCmd := fmt.Sprintf("docker exec %s curl -sf http://localhost%s", o.config.Name, healthPath)
	result, err = o.client.Exec(healthCmd)
	if err != nil || result.ExitCode != 0 {
		return fmt.Errorf("health endpoint check failed")
	}

	return nil
}

func (o *Orchestrator) activateRelease(releasePath, currentPath string) error {
	commands := []string{
		fmt.Sprintf("ln -sfn %s %s", releasePath, currentPath),
		fmt.Sprintf("echo '%s' > %s/release", o.tag, releasePath),
	}

	return o.runCommands(commands)
}

func (o *Orchestrator) runHooks(hooks []string) error {
	for _, hook := range hooks {
		if err := security.ValidateDockerCommand(hook); err != nil {
			return fmt.Errorf("invalid hook command %q: %w", hook, err)
		}
		cmd := fmt.Sprintf("docker exec %s %s", o.config.Name, hook)
		result, err := o.client.Exec(cmd)
		if err != nil {
			return fmt.Errorf("hook '%s' failed: %w", hook, err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("hook '%s' failed: %s", hook, result.Stderr)
		}
	}
	return nil
}

func (o *Orchestrator) cleanupReleases() {
	keepReleases := o.config.Deploy.KeepReleases
	if keepReleases <= 0 {
		keepReleases = 5
	}

	cleanupCmd := fmt.Sprintf(
		"cd %s/releases && ls -1t | tail -n +%d | xargs -r rm -rf",
		o.basePath, keepReleases+1)

	_, _ = o.client.Exec(cleanupCmd)
}

func (o *Orchestrator) runCommands(commands []string) error {
	for _, cmd := range commands {
		if o.verbose {
			o.message(fmt.Sprintf("  > %s", cmd))
		}
		result, err := o.client.Exec(cmd)
		if err != nil {
			return err
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("command failed: %s", result.Stderr)
		}
	}
	return nil
}

// Rollback rolls back to a previous release
func (o *Orchestrator) Rollback(targetRelease string) error {
	if targetRelease == "" {
		// Get previous release
		result, err := o.client.Exec(fmt.Sprintf("ls -1t %s/releases | head -2 | tail -1", o.basePath))
		if err != nil {
			return fmt.Errorf("failed to get releases: %w", err)
		}
		targetRelease = strings.TrimSpace(result.Stdout)
		if targetRelease == "" {
			return fmt.Errorf("no previous release available")
		}
	}

	releasePath := filepath.Join(o.basePath, "releases", targetRelease)
	currentPath := filepath.Join(o.basePath, "current")

	// Verify target release exists
	result, _ := o.client.Exec(fmt.Sprintf("test -d %s && echo 'exists'", releasePath))
	if !strings.Contains(result.Stdout, "exists") {
		return fmt.Errorf("release '%s' not found", targetRelease)
	}

	// Stop current container
	o.stopContainer()

	// Start the target release
	imageName := fmt.Sprintf("%s:%s", o.config.Name, targetRelease)
	if err := o.startContainer(imageName); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Update symlink
	return o.activateRelease(releasePath, currentPath)
}
