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
- Detect API Platform and set the health check path to `/api` automatically

This creates a `frankendeploy.yaml` configuration file.

## Step 2: Generate Docker Files (optional)

```bash
frankendeploy build
```

This generates:
- `Dockerfile` - Multi-stage build with FrankenPHP
- `docker-entrypoint.sh` - Startup script (waits for DB, runs migrations)
- `compose.yaml` - Development environment
- `compose.prod.yaml` - Production template
- `.dockerignore` - Optimized ignore patterns

This step is optional before deploying: `frankendeploy deploy` generates any missing Docker artifacts automatically (and never overwrites files you have customized). Run `build` explicitly when you want to inspect or customize the generated files, or to use the local dev environment.

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
frankendeploy env set production APP_SECRET="your-secret-key"
```

**No `DATABASE_URL` needed with a managed database**: if your `frankendeploy.yaml` has `database.managed: true` (the default for PostgreSQL/MySQL), FrankenDeploy creates the database container and injects `DATABASE_URL` automatically. Only set it yourself when using an external database:

```bash
frankendeploy env set production DATABASE_URL="postgresql://user:pass@host/db"
```

Or push a local `.env.prod` file:

```bash
frankendeploy env push production .env.prod
```

## Step 6: Deploy

```bash
frankendeploy deploy production --remote-build
```

The `--remote-build` flag builds the Docker image directly on the server (recommended for Apple Silicon Macs). FrankenDeploy also detects architecture mismatches automatically and offers to enable remote build for you.

That's it! Your Symfony app is now live with:
- FrankenPHP as the application server
- Automatic HTTPS via Caddy
- Zero-downtime blue-green deployments with health checks and rollback

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
