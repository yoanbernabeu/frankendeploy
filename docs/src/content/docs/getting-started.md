---
title: Introduction
description: What is FrankenDeploy and why use it
---

## What is FrankenDeploy?

**FrankenDeploy** is a CLI that deploys Symfony applications to any VPS using [FrankenPHP](https://frankenphp.dev).

It covers the whole path from local development to production: Docker configuration, server preparation, zero-downtime deployments, rollbacks. One YAML file, a handful of commands, no PaaS.

## Built on FrankenPHP

FrankenDeploy is a deployment layer on top of **FrankenPHP**, the modern PHP application server created by Kévin Dunglas. FrankenPHP combines:

- **Caddy web server** - Automatic HTTPS, HTTP/2, HTTP/3
- **Worker mode** - Keeps your app in memory for ultra-fast responses
- **Single binary** - No separate PHP-FPM, nginx, or Apache needed

FrankenDeploy wraps all this into simple commands.

## What it does for you

### Auto-detection
`frankendeploy init` analyzes your Symfony project and detects:
- PHP version from `composer.json` (8.2 minimum, required by FrankenPHP)
- Required PHP extensions (from `composer.json`, plus what your database and Messenger transports need)
- Database driver (PostgreSQL, MySQL, MariaDB, SQLite) from `.env` / `.env.local`
- Asset build tool (Webpack Encore, Vite, AssetMapper, Tailwind bundle)
- Symfony Messenger transports, Mailer, Scheduler
- API Platform (sets the health check path to `/api`)
- FrankenPHP worker mode when your runtime supports it

### Docker generation
Generates an optimized multi-stage `Dockerfile` for FrankenPHP, a `compose.yaml` for local development, and the supporting files. Missing files are generated at deploy time; files you customized are never overwritten.

### Server preparation
`frankendeploy server setup` turns a bare Ubuntu/Debian VPS into a deployment target: Docker, UFW firewall, Fail2ban, and Caddy as a front reverse proxy with automatic Let's Encrypt certificates. `frankendeploy doctor` checks everything (local Docker, SSH, server, DNS) and tells you exactly what to fix before you deploy.

### One-command, zero-downtime deployment
```bash
frankendeploy deploy prod
```
Builds the image (locally or on the server), transfers it, starts the new version next to the old one, runs your migrations, checks the application health, then switches traffic. If anything fails before the switch, the old version keeps serving. A managed database, its credentials and a backup before each migration are handled for you.

### Operations
- `rollback` to any kept release, with the same health check as a deploy
- `env set` / `env push` for production secrets, applied without downtime with `--reload`
- `logs`, `exec`, `shell` on the running container
- Several applications on one VPS, each on its own isolated Docker network

## Quick Example

```bash
# In your Symfony project
cd my-symfony-app

# Analyze the project and create frankendeploy.yaml
frankendeploy init --domain my-app.com

# Optional: start the local development environment
frankendeploy build
frankendeploy dev up

# Declare and prepare a server (one-time)
frankendeploy server add prod deploy@my-vps.com
frankendeploy server setup prod --email admin@example.com
frankendeploy doctor prod

# Production secrets for this app
# (DATABASE_URL is injected automatically with a managed database)
openssl rand -hex 32 | frankendeploy env set prod APP_SECRET --from-stdin

# Deploy
frankendeploy deploy prod
```

## Next Steps

- [Installation](/frankendeploy/installation/) - Install FrankenDeploy on your system
- [Quick Start](/frankendeploy/quickstart/) - Get up and running in 5 minutes
- [Project Configuration](/frankendeploy/guides/configuration/) - Customize your deployment
