<div align="center">

# FrankenDeploy

### Deploy Symfony apps like a pro

[![GitHub stars](https://img.shields.io/github/stars/yoanbernabeu/frankendeploy?style=flat&logo=github)](https://github.com/yoanbernabeu/frankendeploy/stargazers)
[![Downloads](https://img.shields.io/github/downloads/yoanbernabeu/frankendeploy/total?style=flat&logo=github)](https://github.com/yoanbernabeu/frankendeploy/releases)
[![Go](https://github.com/yoanbernabeu/frankendeploy/actions/workflows/ci.yml/badge.svg)](https://github.com/yoanbernabeu/frankendeploy/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/yoanbernabeu/frankendeploy)](https://goreportcard.com/report/github.com/yoanbernabeu/frankendeploy)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**From local to production in minutes.**

[Documentation](https://yoanbernabeu.github.io/frankendeploy/) · [Installation](#installation) · [Quick Start](#quick-start)

</div>

---

`frankendeploy` is a CLI that deploys Symfony applications to any VPS. It auto-detects your project configuration, generates optimized Docker files, and handles the entire deployment pipeline—SSL, health checks, rollbacks included.

**Built on FrankenPHP**, the modern PHP app server by Kévin Dunglas.

> ⚠️ **Experimental** — FrankenDeploy is currently in experimental phase. Breaking changes may occur between versions. Use with caution in production environments.

## Features

- **Zero config** — Auto-detects PHP version, extensions, database, assets, Messenger, worker mode
- **One command deploy** — `frankendeploy deploy prod` and you're live
- **Automatic HTTPS** — Let's Encrypt certificates via Caddy
- **Zero downtime** — Blue-green deployments with health checks; a failed deploy never takes the site down
- **Managed database** — PostgreSQL, MySQL or MariaDB container, credentials injected, backup before every migration
- **Instant rollback** — `frankendeploy rollback prod`, with the same health check as a deploy
- **Server preparation** — Docker, firewall, Fail2ban, Caddy, and `frankendeploy doctor` to check everything before you deploy
- **Isolated apps** — Several apps on one VPS, each on its own Docker network
- **Local dev included** — Same Docker image for dev and prod

## Installation

**Homebrew (macOS/Linux):**
```bash
brew install yoanbernabeu/tap/frankendeploy
```

**Install script (Linux/macOS):**
```bash
curl -fsSL https://raw.githubusercontent.com/yoanbernabeu/frankendeploy/main/scripts/install.sh | sh
```

**Go:**
```bash
go install github.com/yoanbernabeu/frankendeploy/cmd/frankendeploy@latest
```

**Requirements:** Docker (local dev), SSH access to a VPS (deployment).

## Quick Start

```bash
# In your Symfony project
frankendeploy init --domain my-app.com                # Analyze & configure

# Prepare your server (one-time)
frankendeploy server add prod user@my-vps.com
frankendeploy server setup prod --email you@email.com
frankendeploy doctor prod                             # Server, DNS, all good?

# Production secrets (DATABASE_URL is handled for you with a managed database)
openssl rand -hex 32 | frankendeploy env set prod APP_SECRET --from-stdin

# Deploy
frankendeploy deploy prod                             # That's it
```

Your app is now live at `https://my-app.com` with automatic HTTPS.

## Why FrankenDeploy?

Stop paying $20+/month for PaaS when a $5 VPS handles more traffic than you'll ever need.

| | PaaS (Heroku, etc.) | FrankenDeploy |
|---|---|---|
| **Cost** | $20-50+/month | $5/month VPS |
| **Setup** | Vendor-specific config | One YAML file |
| **Control** | Limited | Full root access |
| **Vendor lock-in** | Yes | No |
| **SSL** | Included | Included (Let's Encrypt) |

**Ideal for:** Side projects, freelance work, early-stage startups.

## Commands

| Command | Description |
|---------|-------------|
| `init` | Analyze the project and generate `frankendeploy.yaml` |
| `build` | Generate the Dockerfile and compose files |
| `dev up/down/logs/restart` | Local development environment |
| `server add/setup/list/status/set/remove` | Manage deployment servers |
| `doctor` | Check the local machine, the server and the DNS before a deploy |
| `deploy` | Blue-green deployment |
| `rollback` | Go back to a previous release |
| `app list/status/remove` | Manage the applications deployed on a server |
| `env set/get/list/remove/push/pull` | Manage production environment variables |
| `logs` | Application and worker logs |
| `exec` / `shell` | Run a command or open a shell in the container |

## Documentation

- **[Quick Start](https://yoanbernabeu.github.io/frankendeploy/quickstart/)** — First deployment walkthrough
- **[frankendeploy.yaml reference](https://yoanbernabeu.github.io/frankendeploy/config/project/)** — Every option explained
- **[Deployment guide](https://yoanbernabeu.github.io/frankendeploy/guides/deployment/)** — Health checks, hooks, backups, CI/CD
- **[CLI Reference](https://yoanbernabeu.github.io/frankendeploy/commands/frankendeploy/)** — Every command documented

## Contributing

Contributions welcome! See the repo issues or open a PR.

## License

[MIT License](LICENSE) — Yoan Bernabeu 2025
