---
title: Quick Start
description: Deploy your Symfony app in 5 minutes
---

## Prerequisites

- A Symfony project
- Docker installed locally
- A VPS with SSH access (for production deployment)

## Step 1: Initialize Your Project

Navigate to your Symfony project and run:

```bash
cd my-symfony-app
frankendeploy init
```

You can also specify your production domain directly:

```bash
frankendeploy init --domain my-app.com
```

FrankenDeploy will:
- Detect your PHP version
- Identify required extensions
- Find your database configuration
- Detect your asset build system

This creates a `frankendeploy.yaml` configuration file.

## Step 2: Generate Docker Files

```bash
frankendeploy build
```

This generates:
- `Dockerfile` - Multi-stage build with FrankenPHP
- `docker-compose.yaml` - Development environment
- `docker-compose.prod.yaml` - Production template
- `.dockerignore` - Optimized ignore patterns

## Step 3: Start Local Development

```bash
frankendeploy dev up
```

Your app is now running at **http://localhost:8000**

Other dev commands:
```bash
frankendeploy dev logs   # View logs
frankendeploy dev down   # Stop environment
frankendeploy dev restart # Restart
```

## Step 4: Configure a Server

Add your VPS to FrankenDeploy:

```bash
frankendeploy server add production deploy@your-vps.com
```

FrankenDeploy will automatically test the SSH connection and find the right key.

Options:
- `--port 2222` - Custom SSH port (default: 22)
- `--key ~/.ssh/id_rsa` - SSH private key path (auto-detected if not specified)
- `--skip-test` - Skip SSH connection test

Then set up the server (installs Docker, Caddy, etc.):

```bash
frankendeploy server setup production --email admin@example.com
```

The `--email` is required for Let's Encrypt SSL certificates.

## Step 5: Configure Environment Variables

Set your production secrets for this application (run from your project directory):

```bash
frankendeploy env set production DATABASE_URL="postgresql://user:pass@host/db"
frankendeploy env set production APP_SECRET="your-secret-key"
```

Or push a local `.env.prod` file:

```bash
frankendeploy env push production .env.prod
```

## Step 6: Deploy

```bash
frankendeploy deploy production --remote-build
```

The `--remote-build` flag builds the Docker image directly on the server (recommended for Apple Silicon Macs).

That's it! Your Symfony app is now live with:
- FrankenPHP worker mode for performance
- Automatic HTTPS via Caddy
- Health checks and rollback support

## Common Operations

### View Production Logs
```bash
frankendeploy logs production
```

### Execute Commands
```bash
frankendeploy exec production php bin/console cache:clear
```

### Open Shell
```bash
frankendeploy shell production
```

### Rollback
```bash
frankendeploy rollback production
```

### Update Environment Variables (Zero Downtime)
Update an environment variable for your application and apply it immediately:
```bash
frankendeploy env set production NEW_VAR=value --reload
```

## Next Steps

- [Environment Variables](/frankendeploy/guides/environment-variables/) - Managing secrets and configuration
- [Configuration Guide](/frankendeploy/guides/configuration/) - Customize your setup
- [Deployment Guide](/frankendeploy/guides/deployment/) - Advanced deployment options
