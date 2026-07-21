---
title: Server Setup
description: Configure your VPS for FrankenDeploy deployments
---

## Requirements

Your VPS should have:
- Ubuntu 22.04+ or Debian 11+ (recommended)
- SSH access with key-based authentication
- At least 1GB RAM
- Port 80 and 443 open (FrankenDeploy configures UFW automatically)

## Adding a Server

```bash
frankendeploy server add production deploy@your-server.com
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
frankendeploy server add prod deploy@51.210.xx.xx
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
frankendeploy server setup production --email admin@example.com
```

The `--email` flag is **required** for Let's Encrypt certificate registration.

This command:
1. Installs Docker if not present
2. Configures UFW firewall (HTTP/HTTPS + SSH — both your configured SSH port and the port the SSH daemon actually uses are allowed before the firewall is enabled, so a custom port or gateway setup can never lock you out)
3. Installs and configures Fail2ban (SSH brute-force protection)
4. Creates the FrankenDeploy directory structure
5. Sets up the `frankendeploy` Docker network
6. Deploys Caddy as a Docker container (reverse proxy)

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

All containers are connected via the `frankendeploy` Docker network.

## Architecture

```
                    ┌──────────────────────────────────────┐
                    │               VPS                    │
                    │                                      │
  Internet          │   ┌────────────────────────────┐    │
    │               │   │     Caddy (Docker)         │    │
    │               │   │  ┌──────────────────────┐  │    │
    ├── :443 ──────►│   │  │ Auto HTTPS (Let's    │  │    │
    └── :80  ──────►│   │  │ Encrypt)             │  │    │
                    │   │  └──────────────────────┘  │    │
                    │   │              │             │    │
                    │   │   import apps/*.caddy     │    │
                    │   └──────────────┬─────────────┘    │
                    │                  │                   │
                    │     Docker Network: frankendeploy    │
                    │                  │                   │
                    │     ┌────────────┴────────────┐      │
                    │     │                         │      │
                    │  ┌──▼───┐    ┌──────┐    ┌───▼──┐   │
                    │  │ App1 │    │ App2 │    │ App3 │   │
                    │  │ :80  │    │ :80  │    │ :80  │   │
                    │  └──────┘    └──────┘    └──────┘   │
                    │                                      │
                    └──────────────────────────────────────┘
```

## Verifying Setup

Check the server status:

```bash
frankendeploy server status production
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

## Security Features

FrankenDeploy automatically configures:

1. **UFW Firewall** - Only SSH (your actual SSH ports, not just 22), 80 and 443 open
2. **Fail2ban** - SSH brute-force protection (automatic)

### Additional Recommendations

1. **Disable root login** - Use a deploy user
2. **SSH keys only** - Disable password authentication
3. **Automatic updates** - Enable unattended-upgrades

## Multiple Environments

You can add multiple servers for different environments:

```bash
frankendeploy server add staging deploy@staging.example.com
frankendeploy server add production deploy@prod.example.com

# Setup both
frankendeploy server setup staging --email dev@example.com
frankendeploy server setup production --email admin@example.com

# Deploy to each
frankendeploy deploy staging
frankendeploy deploy production
```
