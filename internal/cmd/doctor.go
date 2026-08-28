package cmd

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/constants"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor <server>",
	Short: "Diagnose the local machine, the server, and DNS before a deploy",
	Long: `Runs preflight checks and explains how to fix every problem found:

Local checks:
- Docker CLI installed and daemon reachable

Remote checks (over SSH):
- passwordless sudo (unless root)
- Docker installed and usable without sudo
- 'frankendeploy' network present
- Caddy reverse proxy container running
- free disk space

DNS check (when a domain is configured or passed via --domain):
- the domain resolves to the server's public IP — the #1 cause of
  Let's Encrypt certificate failures`,
	Args: cobra.ExactArgs(1),
	RunE: runDoctor,
}

var doctorDomain string

func init() {
	rootCmd.AddCommand(doctorCmd)
	doctorCmd.Flags().StringVar(&doctorDomain, "domain", "", "Domain to check against the server IP (default: deploy.domain from frankendeploy.yaml)")
}

// doctorResult is the outcome of a single check.
type doctorResult struct {
	Name string
	OK   bool
	// Detail is shown next to the check name (value observed).
	Detail string
	// Advice explains how to fix a failed check.
	Advice string
	// Warning marks a non-blocking result (shown as ⚠, does not fail doctor).
	Warning bool
}

func runDoctor(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	serverName := args[0]

	results := []doctorResult{}

	// --- Local checks (before connecting: they fail fast) ---
	results = append(results, checkLocalDocker())

	// --- Remote checks ---
	conn, err := ConnectToServerNoProject(serverName)
	if err != nil {
		results = append(results, doctorResult{
			Name:   "SSH connection",
			OK:     false,
			Detail: err.Error(),
			Advice: fmt.Sprintf("Check the server entry with 'frankendeploy server list' and your SSH key.\nAdd or fix it with: frankendeploy server add %s <user@host>", serverName),
		})
		printDoctorReport(results)
		return fmt.Errorf("doctor found blocking problems")
	}
	defer conn.Client.Close()
	client := conn.Client

	results = append(results,
		doctorResult{Name: "SSH connection", OK: true, Detail: fmt.Sprintf("%s@%s:%d", conn.Server.User, conn.Server.Host, conn.Server.Port)},
		checkRemoteSudo(ctx, client),
		checkRemoteDocker(ctx, client),
		checkDockerNetwork(ctx, client),
		checkCaddyRunning(ctx, client),
		checkDiskSpace(ctx, client),
	)

	// --- DNS check ---
	domain := doctorDomain
	if domain == "" {
		if cfg, err := config.LoadProjectConfig(GetConfigFile()); err == nil {
			domain = cfg.Deploy.Domain
		}
	}
	if domain != "" {
		serverIP := detectServerPublicIP(ctx, client, conn.Server.Host)
		if serverIP == "" {
			results = append(results, doctorResult{
				Name:    "DNS " + domain,
				OK:      false,
				Warning: true,
				Detail:  "could not determine the server public IP",
				Advice:  "Check manually that the domain's A record points to the server.",
			})
		} else {
			results = append(results, checkDomainDNS(domain, serverIP, net.LookupHost))
		}
	} else {
		results = append(results, doctorResult{
			Name:    "DNS",
			OK:      true,
			Warning: true,
			Detail:  "skipped (no domain configured — set deploy.domain or pass --domain)",
		})
	}

	printDoctorReport(results)

	for _, r := range results {
		if !r.OK && !r.Warning {
			return fmt.Errorf("doctor found blocking problems")
		}
	}
	PrintSuccess("Everything looks good — ready to deploy!")
	return nil
}

func printDoctorReport(results []doctorResult) {
	fmt.Println()
	for _, r := range results {
		switch {
		case r.OK && r.Warning:
			PrintWarning("%s — %s", r.Name, r.Detail)
		case r.OK:
			detail := r.Detail
			if detail != "" {
				detail = " — " + detail
			}
			PrintSuccess("%s%s", r.Name, detail)
		case r.Warning:
			PrintWarning("%s — %s", r.Name, r.Detail)
			printAdvice(r.Advice)
		default:
			PrintError("%s — %s", r.Name, r.Detail)
			printAdvice(r.Advice)
		}
	}
	fmt.Println()
}

func printAdvice(advice string) {
	if advice == "" {
		return
	}
	for _, line := range strings.Split(advice, "\n") {
		fmt.Printf("      %s\n", line)
	}
}

// checkLocalDocker verifies the local Docker CLI and daemon.
func checkLocalDocker() doctorResult {
	if _, err := exec.LookPath("docker"); err != nil {
		return doctorResult{
			Name:   "Local Docker",
			OK:     false,
			Detail: "docker CLI not found",
			Advice: "Install Docker Desktop (macOS/Windows) or Docker Engine (Linux): https://docs.docker.com/get-docker/\nNote: not needed when deploying with remote build (--remote-build).",
			// Local docker is only required for local builds
			Warning: true,
		}
	}
	out, err := exec.Command("docker", "version", "--format", "{{.Server.Version}}").Output()
	if err != nil {
		return doctorResult{
			Name:    "Local Docker",
			OK:      false,
			Detail:  "daemon not reachable",
			Advice:  "Start Docker (open Docker Desktop, or 'sudo systemctl start docker').\nNote: not needed when deploying with remote build (--remote-build).",
			Warning: true,
		}
	}
	return doctorResult{Name: "Local Docker", OK: true, Detail: "daemon " + strings.TrimSpace(string(out))}
}

// checkRemoteSudo verifies passwordless sudo (root passes directly).
func checkRemoteSudo(ctx context.Context, client ssh.Executor) doctorResult {
	if result, err := client.Exec(ctx, "id -u"); err == nil && strings.TrimSpace(result.Stdout) == "0" {
		return doctorResult{Name: "Remote sudo", OK: true, Detail: "connected as root"}
	}
	result, err := client.Exec(ctx, "sudo -n true")
	if err != nil || result.ExitCode != 0 {
		return doctorResult{
			Name:   "Remote sudo",
			OK:     false,
			Detail: "passwordless sudo not available",
			Advice: "Server setup and some deploy operations need non-interactive sudo.\nGrant it with: echo \"$USER ALL=(ALL) NOPASSWD:ALL\" | sudo tee /etc/sudoers.d/$USER",
		}
	}
	return doctorResult{Name: "Remote sudo", OK: true}
}

// checkRemoteDocker verifies Docker on the server, usable without sudo.
func checkRemoteDocker(ctx context.Context, client ssh.Executor) doctorResult {
	result, err := client.Exec(ctx, "docker version --format '{{.Server.Version}}'")
	if err != nil {
		return doctorResult{Name: "Remote Docker", OK: false, Detail: err.Error()}
	}
	if result.ExitCode != 0 {
		stderr := strings.ToLower(result.Stderr)
		if strings.Contains(stderr, "permission denied") {
			return doctorResult{
				Name:   "Remote Docker",
				OK:     false,
				Detail: "installed but not usable without sudo",
				Advice: "Add the user to the docker group: sudo usermod -aG docker $USER (then reconnect).",
			}
		}
		return doctorResult{
			Name:   "Remote Docker",
			OK:     false,
			Detail: "docker not found on the server",
			Advice: "Run 'frankendeploy server setup <name> --email you@example.com' to install Docker and Caddy.",
		}
	}
	return doctorResult{Name: "Remote Docker", OK: true, Detail: strings.TrimSpace(result.Stdout)}
}

// checkDockerNetwork verifies the frankendeploy Docker network exists.
func checkDockerNetwork(ctx context.Context, client ssh.Executor) doctorResult {
	result, err := client.Exec(ctx, fmt.Sprintf("docker network ls --format '{{.Name}}' --filter name=^%s$", constants.NetworkName))
	if err != nil || result.ExitCode != 0 || strings.TrimSpace(result.Stdout) == "" {
		return doctorResult{
			Name:   "Docker network",
			OK:     false,
			Detail: fmt.Sprintf("network %q missing", constants.NetworkName),
			Advice: fmt.Sprintf("Run 'frankendeploy server setup <name> --email you@example.com', or create it manually:\n  docker network create %s", constants.NetworkName),
		}
	}
	return doctorResult{Name: "Docker network", OK: true, Detail: constants.NetworkName}
}

// checkCaddyRunning verifies the Caddy front proxy container is up.
func checkCaddyRunning(ctx context.Context, client ssh.Executor) doctorResult {
	result, err := client.Exec(ctx, "docker ps --filter name=^caddy$ --format '{{.Status}}'")
	if err != nil || result.ExitCode != 0 || !strings.Contains(result.Stdout, "Up") {
		return doctorResult{
			Name:   "Caddy proxy",
			OK:     false,
			Detail: "caddy container not running",
			Advice: "Run 'frankendeploy server setup <name> --email you@example.com' to (re)start the reverse proxy.",
		}
	}
	return doctorResult{Name: "Caddy proxy", OK: true, Detail: strings.TrimSpace(result.Stdout)}
}

// minFreeDiskKB is the free-space floor: below 2GB a Docker build or image
// transfer is likely to fail mid-deploy.
const minFreeDiskKB = 2 * 1024 * 1024

// checkDiskSpace warns when the root filesystem is nearly full.
func checkDiskSpace(ctx context.Context, client ssh.Executor) doctorResult {
	result, err := client.Exec(ctx, "df -Pk / | tail -1")
	if err != nil || result.ExitCode != 0 {
		return doctorResult{Name: "Disk space", OK: true, Warning: true, Detail: "could not check"}
	}
	// Keep only the data line (df may include its header)
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 5 {
		return doctorResult{Name: "Disk space", OK: true, Warning: true, Detail: "could not parse df output"}
	}
	availKB, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return doctorResult{Name: "Disk space", OK: true, Warning: true, Detail: "could not parse df output"}
	}
	availGB := float64(availKB) / 1024 / 1024
	if availKB < minFreeDiskKB {
		return doctorResult{
			Name:   "Disk space",
			OK:     false,
			Detail: fmt.Sprintf("only %.1f GB free (%s used)", availGB, fields[4]),
			Advice: "A Docker build or image transfer needs room. Free space with:\n  docker system prune -a   (removes unused images)\nOld releases are pruned automatically according to keep_releases.",
		}
	}
	return doctorResult{Name: "Disk space", OK: true, Detail: fmt.Sprintf("%.1f GB free (%s used)", availGB, fields[4])}
}

// detectServerPublicIP asks the server for its public IP — the SSH host in
// the config can be a gateway (NAT/jump host) whose IP differs from the
// node's public one. Falls back to resolving the configured host.
func detectServerPublicIP(ctx context.Context, client ssh.Executor, configuredHost string) string {
	// hostname -I lists the node's addresses; a public one may be among them
	if result, err := client.Exec(ctx, "curl -4 -s --max-time 5 https://ifconfig.me 2>/dev/null || true"); err == nil {
		ip := strings.TrimSpace(result.Stdout)
		if net.ParseIP(ip) != nil {
			return ip
		}
	}
	if addrs, err := net.LookupHost(configuredHost); err == nil && len(addrs) > 0 {
		return addrs[0]
	}
	return ""
}

// checkDomainDNS resolves the domain and compares with the server IP.
// A mismatch is the #1 cause of Let's Encrypt certificate failures.
func checkDomainDNS(domain, serverIP string, lookup func(string) ([]string, error)) doctorResult {
	addrs, err := lookup(domain)
	if err != nil {
		return doctorResult{
			Name:   "DNS " + domain,
			OK:     false,
			Detail: "domain does not resolve",
			Advice: fmt.Sprintf("Create an A record for %s pointing to %s at your DNS provider.\nDNS changes can take up to a few hours to propagate.", domain, serverIP),
		}
	}
	for _, addr := range addrs {
		if addr == serverIP {
			return doctorResult{Name: "DNS " + domain, OK: true, Detail: "→ " + serverIP}
		}
	}
	return doctorResult{
		Name:   "DNS " + domain,
		OK:     false,
		Detail: fmt.Sprintf("resolves to %s, expected %s", strings.Join(addrs, ", "), serverIP),
		Advice: fmt.Sprintf("Point the A record of %s to %s at your DNS provider — otherwise\nLet's Encrypt cannot issue the certificate and the site stays unreachable.", domain, serverIP),
	}
}
