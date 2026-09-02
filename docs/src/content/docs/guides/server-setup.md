---
title: Server Setup
description: Configure your VPS for FrankenDeploy deployments
---

## Requirements

Your VPS should have:
- Ubuntu or Debian (`server setup` refuses any other distribution with an explicit message; Ubuntu 22.04+ and Debian 12+ are what it is tested on)
- SSH access with key-based authentication, as root or as a user with passwordless `sudo`
- At least 1 GB of RAM (2 GB if you build images on the server)
- Ports 80 and 443 reachable from the Internet (FrankenDeploy opens them in UFW)

Throughout this page the server is called `prod`.

## Adding a Server

```bash
frankendeploy server add prod deploy@your-server.com
```

After adding a server, FrankenDeploy **automatically tests the SSH connection**. If the connection fails, it will:
- **Interactive mode:** List available SSH keys and let you choose one
- **CI/CD mode (`--yes`):** Automatically try available keys until one works

The working key is saved to your configuration.

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `--port` | SSH port | 22 |
| `--key` | Path to SSH private key | Auto-detect |
| `--skip-test` | Skip SSH connection test | false |

### Examples

Standard VPS (auto-detect key):
```bash
frankendeploy server add prod deploy@203.0.113.42
```

Custom SSH port:
```bash
frankendeploy server add prod deploy@gate.example.com --port 2222
```

Specify a key explicitly:
```bash
frankendeploy server add prod user@host --key ~/.ssh/my_custom_key
```

Skip SSH test (useful for CI setup):
```bash
frankendeploy server add prod user@host --skip-test
```

### SSH Authentication

FrankenDeploy authenticates in this order:

1. **ssh-agent** — when `SSH_AUTH_SOCK` is set, keys loaded in the agent are tried first
2. **Key file** — the configured key (`--key`) or auto-detected: `~/.ssh/id_ed25519`, then `~/.ssh/id_rsa`, then other keys in `~/.ssh/`

Passphrase-protected keys are fully supported: the passphrase is prompted when needed and never stored. In non-interactive mode (CI/CD, `--yes`), passphrase-protected keys cannot be prompted — use ssh-agent or `FRANKENDEPLOY_SSH_KEY` instead.

### First Connection (Host Key Verification)

On the first connection to a new server, FrankenDeploy shows the host key fingerprint and asks for confirmation, exactly like OpenSSH:

```
The authenticity of host 'your-server.com' can't be established.
ssh-ed25519 key fingerprint is SHA256:xxxxxxxx...
Are you sure you want to continue connecting (yes/no)?
```

The accepted key is recorded in `~/.ssh/known_hosts` — no need to run a manual `ssh` first.

If the server's host key changes later (server reinstalled or recreated), the connection is refused with a clear message. When the change is expected, remove the old key with:

```bash
ssh-keygen -R your-server.com
```

In CI/CD, set `FRANKENDEPLOY_KNOWN_HOSTS` with the content of your known_hosts file (no interactive confirmation happens there).

## Setting Up the Server

```bash
frankendeploy server setup prod --email admin@example.com
```

The `--email` flag is **required** for Let's Encrypt certificate registration.

This command:
1. Checks the distribution and `sudo` before changing anything
2. Installs Docker if not present
3. Configures UFW firewall (HTTP/HTTPS + SSH — both your configured SSH port and the port the SSH daemon actually uses are allowed before the firewall is enabled, so a custom port or gateway setup can never lock you out)
4. Installs and configures Fail2ban (SSH brute-force protection: 5 attempts, 1 hour ban)
5. Creates the FrankenDeploy directory structure
6. Sets up the `frankendeploy` Docker network (Caddy's home network)
7. Starts Caddy as a Docker container (reverse proxy with automatic HTTPS)

Running it again on an already prepared server is safe: every step checks the current state.

### What Gets Created

```
/opt/frankendeploy/
├── apps/                  # Your deployed applications
└── caddy/
    ├── Caddyfile          # Main Caddy configuration
    ├── apps/              # Per-app Caddy configs (*.caddy)
    └── logs/              # Caddy access logs per app
```

### Docker Containers

| Container | Purpose |
|-----------|---------|
| `caddy` | Reverse proxy with automatic HTTPS |
| `<app-name>` | Your deployed applications |
| `<app-name>-worker` | Messenger worker (when enabled) |
| `<app-name>-db` | Managed database (when enabled) |

### Docker Networks

Each application gets its own network, `frankendeploy-<app-name>`, created by its first deploy. The app, its worker and its database run on it, and Caddy is attached to it so it can still reach `<app-name>:8080` by name. Applications sharing a VPS never see each other's containers: a compromised app cannot reach another app's database.

The `frankendeploy` network created by `server setup` is Caddy's home network. Applications deployed before per-app networks existed are migrated on their next deploy (the database container is moved, never recreated), and `frankendeploy doctor` reports any container still left on the shared network.

Docker's default address pools cover about 30 bridge networks per host, which is far beyond what a single VPS is meant to host. Past that, `deploy` fails with a clear message pointing to the `default-address-pools` setting of `/etc/docker/daemon.json`.

## Architecture

```
                    ┌────────────────────────────────────────────────────────────────────┐
                    │                                VPS                                 │
                    │                                                                    │
  Internet          │   ┌────────────────────────────────────────────────────────────┐   │
    │               │   │                       Caddy (Docker)                       │   │
    ├── :443 ──────►│   │                 Auto HTTPS (Let's Encrypt)                 │   │
    └── :80  ──────►│   │                    import apps/*.caddy                     │   │
                    │   └─────────┬────────────────────┬────────────────────┬────────┘   │
                    │             │                    │                    │            │
                    │   ┌─────────▼────────┐ ┌─────────▼────────┐ ┌─────────▼────────┐   │
                    │   │frankendeploy-app1│ │frankendeploy-app2│ │frankendeploy-app3│   │
                    │   │ ┌──────┐ ┌────┐  │ │ ┌──────┐ ┌────┐  │ │ ┌──────┐         │   │
                    │   │ │ app1 │ │ db │  │ │ │ app2 │ │ db │  │ │ │ app3 │         │   │
                    │   │ │:8080 │ └────┘  │ │ │:8080 │ └────┘  │ │ │:8080 │         │   │
                    │   │ └──────┘         │ │ └──────┘         │ │ └──────┘         │   │
                    │   └──────────────────┘ └──────────────────┘ └──────────────────┘   │
                    │    one isolated network per application, Caddy attached to each    │
                    └────────────────────────────────────────────────────────────────────┘
```

## Verifying Setup

Run the preflight checks, which also verify the DNS of your domain:

```bash
frankendeploy doctor prod
```

Every failed check names the command that fixes it. See [Preflight Checks](/frankendeploy/guides/doctor/).

For resource usage, check the server status:

```bash
frankendeploy server status prod
```

This shows:
- Connection status
- Docker version
- Caddy container status
- Docker network status
- **System metrics:**
  - CPU usage
  - Memory usage
  - Disk usage
  - Load average
- **Per-application resource consumption** (CPU and RAM per container)
- Deployed applications

## Managing Servers

### List Servers
```bash
frankendeploy server list
```

### Remove Server
```bash
frankendeploy server remove staging
```

This only removes the server from your local configuration. It does not affect the server itself.

## Manual Server Preparation

If you prefer to set up the server manually:

### Install Docker
```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
```

### Create Directory Structure
```bash
mkdir -p /opt/frankendeploy/{apps,caddy/apps,caddy/logs}
```

### Create Docker Network
```bash
docker network create frankendeploy
```

This is Caddy's home network only: each application gets its own `frankendeploy-<app-name>` network, created automatically by its first deploy.

### Create Caddyfile
```bash
cat > /opt/frankendeploy/caddy/Caddyfile << 'EOF'
{
    admin localhost:2019
    email your@email.com
    auto_https on
}

import /config/apps/*.caddy
EOF
```

### Start Caddy Container
```bash
docker run -d --name caddy \
  --network frankendeploy \
  --restart unless-stopped \
  -p 80:80 -p 443:443 -p 443:443/udp \
  -v /opt/frankendeploy/caddy/Caddyfile:/etc/caddy/Caddyfile:ro \
  -v /opt/frankendeploy/caddy/apps:/config/apps:ro \
  -v /opt/frankendeploy/caddy/logs:/config/logs \
  -v caddy_data:/data \
  -v caddy_config:/config/caddy \
  caddy:alpine
```

## Firewall Configuration

FrankenDeploy configures UFW automatically. If you need to do it manually:

```bash
sudo ufw allow ssh
sudo ufw allow http
sudo ufw allow https
sudo ufw enable
```

## Zero-Downtime Reload

When you deploy an app, FrankenDeploy:
1. Writes the app's Caddy config to `/opt/frankendeploy/caddy/apps/<app>.caddy`
2. Reloads Caddy without restart: `docker exec caddy caddy reload`

This ensures **zero downtime** for existing apps during deployments.

## Security Model

What FrankenDeploy does on the server, and what it deliberately leaves to you.

### What `server setup` does

- **UFW firewall**: only SSH (your actual SSH ports, not just 22), 80 and 443 are open; everything else is denied
- **Fail2ban** on SSH: 5 failed attempts, 1 hour ban
- **Caddy** is the only container with published ports. Its admin API listens on `localhost` inside the container and is never exposed

### What every deploy does

- The application runs as a **non-root user** on an unprivileged port (8080)
- **No port is published** for the app, the worker or the database: the only way in is Caddy, on 80 and 443
- **TLS** with automatic Let's Encrypt certificates, HSTS enabled; Symfony trusts only Caddy as a proxy (`SYMFONY_TRUSTED_PROXIES` set to the private subnets), so it sees the real client IP and the HTTPS scheme
- **One Docker network per application**: apps sharing a VPS cannot reach each other's containers
- **Secrets** live in `.env.local` on the server, `chmod 600`, mounted read-only; `env set --from-stdin` keeps them out of your shell history
- **Managed databases** get random credentials, a random one-time root password (MySQL/MariaDB), and a dump before every migration
- **Log rotation** on every container, so logs cannot fill the disk

### What it does not do

FrankenDeploy prepares a server; it is not a hardening tool.

- **No automatic security updates**: install `unattended-upgrades` or update the system yourself
- **No `sshd` hardening**: password authentication and root login stay as your distribution ships them. Disable password authentication once your key works
- **No encryption of secrets at rest**: `.env.local` is readable by root and by anyone with SSH access. Use your provider's disk encryption for backups and snapshots
- **No intrusion detection, no container hardening beyond non-root**

A `server harden` command covering updates, `sshd` and an audit in `doctor` is [being discussed](https://github.com/yoanbernabeu/frankendeploy/issues/95).

## Multiple Environments

You can add multiple servers for different environments:

```bash
frankendeploy server add staging deploy@staging.example.com
frankendeploy server add prod deploy@203.0.113.42

# Setup both
frankendeploy server setup staging --email dev@example.com
frankendeploy server setup prod --email admin@example.com

# Deploy to each
frankendeploy deploy staging
frankendeploy deploy prod
```
