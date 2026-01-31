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

- **Zero config** — Auto-detects PHP version, extensions, database, and assets
- **One command deploy** — `frankendeploy deploy prod` and you're live
- **Automatic HTTPS** — Let's Encrypt certificates via Caddy
- **Zero downtime** — Rolling deployments with health checks
- **Instant rollback** — `frankendeploy rollback prod` if something goes wrong
- **Local dev included** — Same Docker setup for dev and prod

## Installation

**Linux/macOS:**
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
frankendeploy init                                    # Analyze & configure

# Setup your server (one-time)
frankendeploy server add prod user@my-vps.com
frankendeploy server setup prod --email you@email.com

# Deploy
frankendeploy deploy prod                             # That's it
```

Your app is now live at `https://your-domain.com` with automatic HTTPS.

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
| `init` | Analyze project and generate config |
| `build` | Generate Dockerfile and compose files |
| `dev up/down/logs` | Local development environment |
| `server add/setup/list` | Manage deployment servers |
| `deploy` | Deploy to production |
| `rollback` | Revert to previous release |
| `logs` | View application logs |
| `exec` / `shell` | Run commands in container |
| `env set/get/list` | Manage environment variables |

## Documentation

- **[Getting Started](https://yoanbernabeu.github.io/frankendeploy/getting-started/)** — First deployment walkthrough
- **[Configuration](https://yoanbernabeu.github.io/frankendeploy/guides/configuration/)** — All YAML options explained
- **[CLI Reference](https://yoanbernabeu.github.io/frankendeploy/commands/frankendeploy/)** — Every command documented

## Contributing

Contributions welcome! See the repo issues or open a PR.

## License

[MIT License](LICENSE) — Yoan Bernabeu 2025
