---
title: Quick Start
description: Deploy your Symfony app in 5 minutes
---

## Prerequisites

- A Symfony project
- Docker installed locally (only needed for local builds and the dev environment)
- A VPS running Ubuntu or Debian, reachable over SSH with a key

Throughout the docs the server is called `prod`. Name yours as you like.

## Step 1: Initialize Your Project

Navigate to your Symfony project and run:

```bash
cd my-symfony-app
frankendeploy init --domain my-app.com
```

FrankenDeploy analyzes the project (PHP version and extensions, database, assets, Messenger, Mailer, API Platform, worker mode) and writes a `frankendeploy.yaml`. Every inference is printed, so you can check what was detected. The domain can also be added later under `deploy.domain`.

## Step 2: Generate Docker Files (optional)

```bash
frankendeploy build
```

This generates:
- `Dockerfile` - Multi-stage build with FrankenPHP
- `docker-entrypoint.sh` - Startup script (waits for the database to accept connections)
- `compose.yaml` - Local development environment
- `compose.prod.yaml` - Production template, for people who deploy with Compose by hand
- `.dockerignore` - Optimized ignore patterns
- `Caddyfile` - Only with FrankenPHP worker mode enabled

This step is optional before deploying: `frankendeploy deploy` generates any missing file and never overwrites one you customized. Run `build` explicitly to inspect or customize the generated files, or to use the local dev environment.

## Step 3: Start Local Development (optional)

```bash
frankendeploy dev up
```

Your app is now running at **http://localhost:8000**, with its database, Mailpit and RabbitMQ when your project needs them.

```bash
frankendeploy dev logs      # View logs
frankendeploy dev down      # Stop the environment
frankendeploy dev restart   # Restart it
```

## Step 4: Declare and Prepare a Server

Add your VPS to FrankenDeploy:

```bash
frankendeploy server add prod deploy@your-vps.com
```

FrankenDeploy tests the SSH connection right away and finds the right key. Options: `--port 2222` for a custom SSH port, `--key ~/.ssh/my_key` to force a key, `--skip-test` to skip the connection test.

Then prepare the server (Docker, firewall, Fail2ban, Caddy):

```bash
frankendeploy server setup prod --email admin@example.com
```

The `--email` is required for Let's Encrypt certificates.

Finally, check that everything is in place, including the DNS record of your domain:

```bash
frankendeploy doctor prod
```

Every failed check comes with the command that fixes it. See [Preflight checks](/frankendeploy/guides/doctor/).

## Step 5: Configure Production Secrets

Secrets live on the server, per application, in a `.env.local` file. Set them from your project directory:

```bash
openssl rand -hex 32 | frankendeploy env set prod APP_SECRET --from-stdin
```

**No `DATABASE_URL` needed with a managed database**: when `frankendeploy.yaml` has `database.managed: true` (the default for PostgreSQL, MySQL and MariaDB), FrankenDeploy creates the database container and injects `DATABASE_URL` automatically. Only set it yourself for an external database:

```bash
frankendeploy env set prod DATABASE_URL --from-stdin
```

You can also push a whole file: `frankendeploy env push prod .env.prod`.

If you forget `APP_SECRET`, the deploy stops before touching the server and offers to generate one for you.

## Step 6: Deploy

```bash
frankendeploy deploy prod
```

From an Apple Silicon Mac to an x86 VPS, FrankenDeploy detects the architecture mismatch and offers to build on the server (`--remote-build`); the choice is remembered.

That's it. Your Symfony app is live with:
- FrankenPHP as the application server
- Automatic HTTPS via Caddy
- Zero-downtime blue-green deployments with health checks
- A managed database with automatic backups before migrations

## Common Operations

```bash
frankendeploy logs prod -f                          # Follow production logs
frankendeploy exec prod php bin/console cache:clear # Run a command in the container
frankendeploy shell prod                            # Open a shell in the container
frankendeploy rollback prod                         # Go back to the previous release
frankendeploy env set prod NEW_VAR=value --reload   # Change a variable without downtime
frankendeploy app status prod                       # Releases and container status
```

## Next Steps

- [Environment Variables](/frankendeploy/guides/environment-variables/) - Managing secrets and configuration
- [Configuration Guide](/frankendeploy/guides/configuration/) - Customize your setup
- [Deployment Guide](/frankendeploy/guides/deployment/) - Advanced deployment options
