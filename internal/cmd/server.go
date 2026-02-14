package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/frankendeploy/internal/caddy"
	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/constants"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Manage deployment servers",
	Long:  `Commands to add, configure, and manage deployment servers.`,
}

var serverAddCmd = &cobra.Command{
	Use:   "add <name> <user@host>",
	Short: "Add a new server",
	Long: `Adds a new server to the global configuration.

Example:
  frankendeploy server add production deploy@my-vps.com
  frankendeploy server add staging user@staging.example.com --port 2222`,
	Args: cobra.ExactArgs(2),
	RunE: runServerAdd,
}

var serverSetupCmd = &cobra.Command{
	Use:   "setup <name>",
	Short: "Setup a server for deployments",
	Long: `Configures a server for FrankenDeploy deployments.

This command will:
- Install Docker if not present
- Configure UFW firewall (ports 22, 80, 443)
- Install and configure Fail2ban (SSH brute-force protection)
- Configure Docker for non-root usage
- Set up the deployment directory structure
- Configure Caddy as reverse proxy`,
	Args: cobra.ExactArgs(1),
	RunE: runServerSetup,
}

var serverListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured servers",
	RunE:  runServerList,
}

var serverStatusCmd = &cobra.Command{
	Use:   "status <name>",
	Short: "Show server status and system metrics",
	Long: `Displays comprehensive information about a server:
- Connection status and Docker version
- System metrics: CPU, Memory, Disk usage, Load average
- Per-application resource consumption (CPU/RAM per container)
- Caddy reverse proxy status
- Deployed applications`,
	Args: cobra.ExactArgs(1),
	RunE: runServerStatus,
}

var serverRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a server",
	Args:  cobra.ExactArgs(1),
	RunE:  runServerRemove,
}

var serverSetCmd = &cobra.Command{
	Use:   "set <server> <key> <value>",
	Short: "Set a server configuration value",
	Long: `Sets a configuration value for a server.

Available keys:
  remote_build  Enable/disable remote build (true/false)

Examples:
  frankendeploy server set prod remote_build true
  frankendeploy server set staging remote_build false`,
	Args: cobra.ExactArgs(3),
	RunE: runServerSet,
}

var (
	serverPort    int
	serverKeyPath string
	setupEmail    string
	skipSSHTest   bool
)

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.AddCommand(serverAddCmd)
	serverCmd.AddCommand(serverSetupCmd)
	serverCmd.AddCommand(serverListCmd)
	serverCmd.AddCommand(serverStatusCmd)
	serverCmd.AddCommand(serverRemoveCmd)
	serverCmd.AddCommand(serverSetCmd)

	serverAddCmd.Flags().IntVarP(&serverPort, "port", "p", 22, "SSH port")
	serverAddCmd.Flags().StringVarP(&serverKeyPath, "key", "k", "", "SSH private key path")
	serverAddCmd.Flags().BoolVar(&skipSSHTest, "skip-test", false, "Skip SSH connection test")

	serverSetupCmd.Flags().StringVarP(&setupEmail, "email", "e", "", "Email for Let's Encrypt certificates (required)")
	_ = serverSetupCmd.MarkFlagRequired("email")
}

func runServerAdd(cmd *cobra.Command, args []string) error {
	name := args[0]
	hostSpec := args[1]

	// Validate server name
	if err := security.ValidateServerName(name); err != nil {
		return fmt.Errorf("invalid server name: %w", err)
	}

	// Parse user@host
	parts := strings.SplitN(hostSpec, "@", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid host format, use user@host")
	}
	user, host := parts[0], parts[1]

	// Load global config
	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		return fmt.Errorf("failed to load global config: %w", err)
	}

	// Create server config
	serverCfg := config.ServerConfig{
		Host:    host,
		User:    user,
		Port:    serverPort,
		KeyPath: serverKeyPath,
	}

	// Validate
	if errors := config.ValidateServerConfig(&serverCfg); errors.HasErrors() {
		return fmt.Errorf("invalid server configuration: %w", errors)
	}

	// Add server
	if err := globalCfg.AddServer(name, serverCfg); err != nil {
		return err
	}

	// Save config
	if err := config.SaveGlobalConfig(globalCfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	PrintSuccess("Added server '%s' (%s@%s)", name, user, host)

	// Skip SSH test if requested
	if skipSSHTest {
		PrintInfo("Skipping SSH connection test (--skip-test)")
		printNextSteps(name)
		return nil
	}

	// Test SSH connection and configure key if needed
	if err := testAndConfigureSSH(name, &serverCfg, globalCfg); err != nil {
		PrintWarning("SSH connection could not be established: %v", err)
		PrintInfo("You can test the connection manually with: ssh %s@%s -p %d", user, host, serverCfg.Port)
	}

	printNextSteps(name)
	return nil
}

func printNextSteps(name string) {
	fmt.Println()
	fmt.Println("Next step:")
	fmt.Printf("  Run 'frankendeploy server setup %s --email your@email.com' to configure the server\n", name)
}

// testAndConfigureSSH tests the SSH connection and tries alternative keys if needed
func testAndConfigureSSH(name string, serverCfg *config.ServerConfig, globalCfg *config.GlobalConfig) error {
	PrintInfo("Testing SSH connection...")

	// Try connection with current configuration
	client := ssh.NewClient(serverCfg.Host, serverCfg.User, serverCfg.Port, serverCfg.KeyPath)
	if err := client.Connect(); err == nil {
		client.Close()
		PrintSuccess("SSH connection successful")
		return nil
	}

	PrintWarning("Connection failed with default key")

	// Discover available SSH keys
	keys, err := ssh.DiscoverSSHKeys()
	if err != nil {
		return fmt.Errorf("failed to discover SSH keys: %w", err)
	}

	// Filter out encrypted keys and already tried key
	var availableKeys []ssh.SSHKeyInfo
	for _, key := range keys {
		if key.IsEncrypted {
			PrintVerbose("Skipping encrypted key: %s", key.Name)
			continue
		}
		if serverCfg.KeyPath != "" && key.Path == serverCfg.KeyPath {
			continue
		}
		availableKeys = append(availableKeys, key)
	}

	if len(availableKeys) == 0 {
		return fmt.Errorf("no SSH keys available to try")
	}

	// Try keys - either interactively or automatically
	var workingKey *ssh.SSHKeyInfo
	if IsInteractive() {
		workingKey = interactiveKeySelection(serverCfg, availableKeys)
	} else {
		workingKey = autoTryKeys(serverCfg, availableKeys)
	}

	if workingKey == nil {
		return fmt.Errorf("no working SSH key found")
	}

	// Update server config with working key
	serverCfg.KeyPath = workingKey.Path
	globalCfg.Servers[name] = *serverCfg

	if err := config.SaveGlobalConfig(globalCfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	PrintSuccess("Updated server config with key: %s", workingKey.Path)
	return nil
}

// interactiveKeySelection prompts the user to select an SSH key
func interactiveKeySelection(serverCfg *config.ServerConfig, keys []ssh.SSHKeyInfo) *ssh.SSHKeyInfo {
	options := make([]string, len(keys))
	for i, key := range keys {
		options[i] = fmt.Sprintf("%s (%s)", key.Name, key.Type)
	}

	fmt.Println()
	PrintInfo("Available SSH keys:")
	choice := PromptSelect("Select SSH key to use:", options)
	if choice < 0 {
		return nil
	}

	selectedKey := &keys[choice]
	PrintInfo("Testing with %s...", selectedKey.Path)

	err := ssh.TryConnect(serverCfg.Host, serverCfg.User, serverCfg.Port, selectedKey.Path)
	if err != nil {
		PrintError("Connection failed: %v", err)
		return nil
	}

	PrintSuccess("Connection successful!")
	return selectedKey
}

// autoTryKeys automatically tries available keys in order
func autoTryKeys(serverCfg *config.ServerConfig, keys []ssh.SSHKeyInfo) *ssh.SSHKeyInfo {
	PrintInfo("Trying available SSH keys automatically...")

	for _, key := range keys {
		PrintVerbose("Trying %s...", key.Name)
		err := ssh.TryConnect(serverCfg.Host, serverCfg.User, serverCfg.Port, key.Path)
		if err == nil {
			PrintSuccess("SSH connection successful with %s", key.Name)
			return &key
		}
	}

	return nil
}

func runServerSetup(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	name := args[0]

	conn, err := ConnectToServerNoProject(name)
	if err != nil {
		return err
	}
	defer conn.Client.Close()
	client := conn.Client

	PrintSuccess("Connected to %s", conn.Server.Host)
	PrintInfo("Setting up server for FrankenDeploy...")

	// Step 1: System update and prerequisites
	PrintInfo("[1/5] Installing prerequisites...")
	prereqCommands := []string{
		"sudo apt-get update -qq",
		"sudo apt-get install -y -qq curl ca-certificates",
	}
	if err := runCommandsWithProgress(ctx, client, prereqCommands); err != nil {
		return err
	}

	// Step 2: Install and configure Fail2ban
	PrintInfo("[2/5] Installing Fail2ban...")
	fail2banCommands := []string{
		// Install Fail2ban
		"sudo apt-get install -y -qq fail2ban",
		// Enable and start Fail2ban
		"sudo systemctl enable fail2ban",
		"sudo systemctl start fail2ban",
	}
	if err := runCommandsWithProgress(ctx, client, fail2banCommands); err != nil {
		return err
	}

	// Create Fail2ban jail configuration for SSH
	fail2banConfig := `[sshd]
enabled = true
port = ssh
filter = sshd
logpath = /var/log/auth.log
maxretry = 5
bantime = 3600
findtime = 600
`
	fail2banConfigCmd := fmt.Sprintf(`sudo tee /etc/fail2ban/jail.local > /dev/null << 'FAIL2BANEOF'
%sFAIL2BANEOF`, fail2banConfig)
	if _, err := client.Exec(ctx, fail2banConfigCmd); err != nil {
		PrintWarning("Failed to configure Fail2ban jail: %v", err)
	} else {
		// Restart Fail2ban to apply configuration
		if _, err := client.Exec(ctx, "sudo systemctl restart fail2ban"); err != nil {
			PrintWarning("Could not restart Fail2ban: %v", err)
		}
	}

	// Step 3: Install Docker
	PrintInfo("[3/5] Installing Docker...")
	dockerCommands := []string{
		// Install Docker if not present
		"which docker || (curl -fsSL https://get.docker.com | sudo sh)",
		// Add user to docker group
		"sudo usermod -aG docker $USER || true",
		// Enable and start Docker
		"sudo systemctl enable docker",
		"sudo systemctl start docker",
	}
	if err := runCommandsWithProgress(ctx, client, dockerCommands); err != nil {
		return err
	}

	// Step 4: Create directory structure and Docker network
	PrintInfo("[4/5] Configuring FrankenDeploy...")
	structureCommands := []string{
		// Create directory structure
		fmt.Sprintf("sudo mkdir -p %s", constants.AppsDir),
		fmt.Sprintf("sudo mkdir -p %s/apps", constants.CaddyDir),
		fmt.Sprintf("sudo mkdir -p %s/logs", constants.CaddyDir),
		fmt.Sprintf("sudo chown -R $USER:$USER %s", constants.BasePath),
		// Create Docker network for apps
		fmt.Sprintf("docker network create %s 2>/dev/null || true", constants.NetworkName),
	}
	if err := runCommandsWithProgress(ctx, client, structureCommands); err != nil {
		return err
	}

	// Generate and upload Caddy main configuration
	caddyGen := caddy.NewConfigGenerator()
	mainConfig, err := caddyGen.GenerateMainConfig(setupEmail)
	if err != nil {
		return fmt.Errorf("failed to generate Caddy config: %w", err)
	}

	// Upload Caddyfile
	uploadCaddyCmd := fmt.Sprintf(`cat > %s/Caddyfile << 'CADDYEOF'
%s
CADDYEOF`, constants.CaddyDir, mainConfig)
	if _, err := client.Exec(ctx, uploadCaddyCmd); err != nil {
		return fmt.Errorf("failed to upload Caddyfile: %w", err)
	}

	// Create empty placeholder for apps import
	if _, err := client.Exec(ctx, fmt.Sprintf("touch %s/apps/.keep", constants.CaddyDir)); err != nil {
		return fmt.Errorf("failed to create apps directory: %w", err)
	}

	// Step 5: Configure firewall and start Caddy container
	PrintInfo("[5/5] Configuring firewall and starting Caddy...")
	firewallCommands := []string{
		// Configure UFW firewall
		"sudo ufw allow 22/tcp || true",
		"sudo ufw allow 80/tcp || true",
		"sudo ufw allow 443/tcp || true",
		"sudo ufw --force enable || true",
	}
	if err := runCommandsWithProgress(ctx, client, firewallCommands); err != nil {
		return err
	}

	// Start Caddy container with Admin API exposed on localhost only
	// Note: Admin API on 2019 is NOT exposed to host - only accessible inside container
	// We use docker exec to reload config
	caddyContainerCmd := fmt.Sprintf(`docker rm -f caddy 2>/dev/null || true && docker run -d \
		--name caddy \
		--network %s \
		--restart unless-stopped \
		-p 80:80 \
		-p 443:443 \
		-p 443:443/udp \
		-v %s/Caddyfile:/etc/caddy/Caddyfile:ro \
		-v %s/apps:/config/apps:ro \
		-v %s/logs:/config/logs \
		-v caddy_data:/data \
		-v caddy_config:/config/caddy \
		caddy:alpine`, constants.NetworkName, constants.CaddyDir, constants.CaddyDir, constants.CaddyDir)

	result, err := client.Exec(ctx, caddyContainerCmd)
	if err != nil {
		return fmt.Errorf("failed to start Caddy container: %w", err)
	}
	if err := result.Err(); err != nil {
		return fmt.Errorf("failed to start Caddy container: %w", err)
	}

	// Verify Caddy is running
	result, err = client.Exec(ctx, "docker ps --filter name=caddy --format '{{.Status}}'")
	if err == nil && strings.Contains(result.Stdout, "Up") {
		PrintSuccess("Caddy container is running")
	} else {
		PrintWarning("Caddy container may not be running properly")
	}

	PrintSuccess("Server '%s' is ready for deployments!", name)
	fmt.Println()
	fmt.Println("Configuration:")
	fmt.Printf("  Email:    %s (for Let's Encrypt)\n", setupEmail)
	fmt.Println("  Caddy:    Docker container with Admin API")
	fmt.Println("  Docker:   Installed with 'frankendeploy' network")
	fmt.Println("  Firewall: Ports 22, 80, 443 open")
	fmt.Println("  Fail2ban: SSH protection enabled (5 retries, 1h ban)")
	fmt.Println()
	fmt.Println("Next step:")
	fmt.Println("  Run 'frankendeploy deploy " + name + "' from your Symfony project")

	return nil
}

// runCommandsWithProgress executes a list of commands with error handling
func runCommandsWithProgress(ctx context.Context, client *ssh.Client, commands []string) error {
	for _, command := range commands {
		PrintVerbose("  > %s", command)
		result, err := client.Exec(ctx, command)
		if err != nil {
			return fmt.Errorf("command failed: %w", err)
		}
		// Allow commands with || true or || to fail gracefully
		if result.ExitCode != 0 && !strings.Contains(command, "|| true") && !strings.Contains(command, "|| ") && !strings.Contains(command, "2>/dev/null") {
			return fmt.Errorf("command %q failed: %w", command, result.Err())
		}
	}
	return nil
}

func runServerList(cmd *cobra.Command, args []string) error {
	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	servers := globalCfg.ListServers()
	if len(servers) == 0 {
		PrintInfo("No servers configured")
		fmt.Println()
		fmt.Println("Add a server with:")
		fmt.Println("  frankendeploy server add <name> <user@host>")
		return nil
	}

	fmt.Println("Configured servers:")
	fmt.Println()
	for _, name := range servers {
		server := globalCfg.Servers[name]
		fmt.Printf("  %s\n", name)
		fmt.Printf("    Host: %s@%s:%d\n", server.User, server.Host, server.Port)
		if server.KeyPath != "" {
			fmt.Printf("    Key:  %s\n", server.KeyPath)
		}
		if server.RemoteBuild != nil {
			fmt.Printf("    Remote Build: %v\n", *server.RemoteBuild)
		}
		fmt.Println()
	}

	return nil
}

func runServerStatus(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	name := args[0]

	conn, err := ConnectToServerNoProject(name)
	if err != nil {
		PrintError("Connection failed: %v", err)
		return nil
	}
	defer conn.Client.Close()
	client := conn.Client

	PrintSuccess("Connection: OK")

	// Check Docker
	result, err := client.Exec(ctx, "docker --version")
	if err == nil && result.ExitCode == 0 {
		PrintSuccess("Docker: %s", strings.TrimSpace(result.Stdout))
	} else {
		PrintWarning("Docker: Not installed")
	}

	// Check FrankenDeploy directory
	result, err = client.Exec(ctx, fmt.Sprintf("test -d %s && echo 'exists'", constants.BasePath))
	if err == nil && strings.Contains(result.Stdout, "exists") {
		PrintSuccess("FrankenDeploy: Configured")
	} else {
		PrintWarning("FrankenDeploy: Not configured (run 'frankendeploy server setup %s')", name)
	}

	// Check Caddy container
	result, err = client.Exec(ctx, "docker ps --filter name=caddy --format '{{.Status}}'")
	if err == nil {
		caddyStatus := strings.TrimSpace(result.Stdout)
		if strings.Contains(caddyStatus, "Up") {
			PrintSuccess("Caddy: %s (Docker)", caddyStatus)
		} else {
			PrintWarning("Caddy: Not running")
		}
	} else {
		PrintWarning("Could not check Caddy status: %v", err)
	}

	// Check Docker network
	result, err = client.Exec(ctx, fmt.Sprintf("docker network inspect %s --format '{{.Name}}' 2>/dev/null", constants.NetworkName))
	if err == nil && strings.Contains(result.Stdout, constants.NetworkName) {
		PrintSuccess("Docker network: %s", constants.NetworkName)
	} else {
		PrintWarning("Docker network: %s not found", constants.NetworkName)
	}

	// System resources
	fmt.Println()
	fmt.Println("System Resources:")

	// CPU usage
	result, err = client.Exec(ctx, "top -bn1 | grep 'Cpu(s)' | awk '{print 100 - $8}' 2>/dev/null || echo 'N/A'")
	if err == nil {
		cpuUsage := strings.TrimSpace(result.Stdout)
		if cpuUsage != "" && cpuUsage != "N/A" {
			fmt.Printf("  CPU:    %s%% used\n", cpuUsage)
		}
	}

	// Memory usage
	result, err = client.Exec(ctx, "free -m | awk 'NR==2{printf \"%.1f/%.1fGB (%.0f%%)\", $3/1024, $2/1024, $3*100/$2}'")
	if err == nil {
		memUsage := strings.TrimSpace(result.Stdout)
		if memUsage != "" {
			fmt.Printf("  Memory: %s\n", memUsage)
		}
	}

	// Disk usage
	result, err = client.Exec(ctx, "df -h / | awk 'NR==2{printf \"%s/%s (%s)\", $3, $2, $5}'")
	if err == nil {
		diskUsage := strings.TrimSpace(result.Stdout)
		if diskUsage != "" {
			fmt.Printf("  Disk:   %s\n", diskUsage)
		}
	}

	// Load average
	result, err = client.Exec(ctx, "uptime | awk -F'load average:' '{print $2}' | xargs")
	if err == nil {
		loadAvg := strings.TrimSpace(result.Stdout)
		if loadAvg != "" {
			fmt.Printf("  Load:   %s\n", loadAvg)
		}
	}

	// List deployed apps with container stats
	result, err = client.Exec(ctx, fmt.Sprintf("ls -1 %s 2>/dev/null", constants.AppsDir))
	if err == nil {
		apps := strings.TrimSpace(result.Stdout)
		if apps != "" {
			fmt.Println()
			fmt.Println("Deployed Applications:")
			fmt.Println()
			for _, app := range strings.Split(apps, "\n") {
				if app == "" {
					continue
				}
				fmt.Printf("  %s:\n", app)

				// Get container stats for app
				statsCmd := fmt.Sprintf("docker stats --no-stream --format '{{.CPUPerc}}\t{{.MemUsage}}' %s 2>/dev/null", app)
				statsResult, statsErr := client.Exec(ctx, statsCmd)
				if statsErr == nil {
					stats := strings.TrimSpace(statsResult.Stdout)
					if stats != "" {
						parts := strings.Split(stats, "\t")
						if len(parts) >= 2 {
							fmt.Printf("    App:    CPU %s, Mem %s\n", parts[0], parts[1])
						}
					} else {
						fmt.Printf("    App:    not running\n")
					}
				} else {
					fmt.Printf("    App:    not running\n")
				}

				// Get worker stats if exists
				workerStatsCmd := fmt.Sprintf("docker stats --no-stream --format '{{.CPUPerc}}\t{{.MemUsage}}' %s-worker 2>/dev/null", app)
				workerResult, workerErr := client.Exec(ctx, workerStatsCmd)
				if workerErr == nil {
					workerStats := strings.TrimSpace(workerResult.Stdout)
					if workerStats != "" {
						parts := strings.Split(workerStats, "\t")
						if len(parts) >= 2 {
							fmt.Printf("    Worker: CPU %s, Mem %s\n", parts[0], parts[1])
						}
					}
				}
			}
		}
	}

	return nil
}

func runServerRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Validate server name
	if err := security.ValidateServerName(name); err != nil {
		return fmt.Errorf("invalid server name: %w", err)
	}

	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	if err := globalCfg.RemoveServer(name); err != nil {
		return err
	}

	if err := config.SaveGlobalConfig(globalCfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	PrintSuccess("Removed server '%s'", name)
	return nil
}

func runServerSet(cmd *cobra.Command, args []string) error {
	serverName := args[0]
	key := args[1]
	value := args[2]

	// Validate server name
	if err := security.ValidateServerName(serverName); err != nil {
		return fmt.Errorf("invalid server name: %w", err)
	}

	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	serverCfg, err := globalCfg.GetServer(serverName)
	if err != nil {
		return err
	}

	switch key {
	case "remote_build":
		boolValue, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid value for remote_build: use 'true' or 'false'")
		}
		serverCfg.RemoteBuild = &boolValue

	default:
		return fmt.Errorf("unknown configuration key: %s\n\nAvailable keys:\n  remote_build  Enable/disable remote build (true/false)", key)
	}

	globalCfg.Servers[serverName] = *serverCfg
	if err := config.SaveGlobalConfig(globalCfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	PrintSuccess("Set %s=%s for server '%s'", key, value, serverName)
	return nil
}
