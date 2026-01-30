---
title: Introduction
description: What is FrankenDeploy and why use it
---

## What is FrankenDeploy?

**FrankenDeploy** is a CLI tool that simplifies deploying Symfony applications on VPS servers using [FrankenPHP](https://frankenphp.dev).

It provides a seamless experience from local development to production deployment, handling Docker configuration, server setup, and zero-downtime deployments.

## Built on FrankenPHP

FrankenDeploy is a deployment layer on top of **FrankenPHP**, the modern PHP application server created by Kévin Dunglas. FrankenPHP combines:

- **Caddy web server** - Automatic HTTPS, HTTP/2, HTTP/3
- **Worker mode** - Keeps your app in memory for ultra-fast responses
- **Single binary** - No separate PHP-FPM, nginx, or Apache needed

FrankenDeploy wraps all this power into simple commands.

## Key Features

### Auto-detection
FrankenDeploy analyzes your Symfony project and automatically detects:
- PHP version from `composer.json`
- Required PHP extensions
- Database driver (PostgreSQL, MySQL, SQLite)
- Asset build tools (Webpack Encore, Vite, AssetMapper)

### Docker Generation
Generates optimized Docker configuration:
- Multi-stage Dockerfile with FrankenPHP
- docker-compose.yaml for local development
- Production-ready configuration

### One-Command Deployment
```bash
frankendeploy deploy production
```
Handles everything: building, transferring, starting containers, health checks, and cleanup.

### Rolling Deployments
- Zero-downtime deployments
- Automatic health checks
- Instant rollback if something fails
- Release history management

## Quick Example

```bash
# In your Symfony project
cd my-symfony-app

# Initialize FrankenDeploy
frankendeploy init

# Generate Docker files
frankendeploy build

# Start local development
frankendeploy dev up

# Configure a server (one-time setup)
frankendeploy server add production deploy@my-vps.com --key ~/.ssh/id_rsa
frankendeploy server setup production --email admin@example.com

# Set environment variables
frankendeploy env set production DATABASE_URL="postgresql://..."

# Deploy!
frankendeploy deploy production --remote-build
```

## Next Steps

- [Installation](/frankendeploy/installation/) - Install FrankenDeploy on your system
- [Quick Start](/frankendeploy/quickstart/) - Get up and running in 5 minutes
- [Project Configuration](/frankendeploy/guides/configuration/) - Customize your deployment
