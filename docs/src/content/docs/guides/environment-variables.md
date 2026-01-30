---
title: Environment Variables
description: Managing environment variables in production
---

## Overview

FrankenDeploy stores environment variables in a shared `.env.local` file on the server. This file is mounted into the container and persists across deployments.

## Setting Variables

```bash
# Set a single variable
frankendeploy env set prod APP_SECRET="your-secret-key"
frankendeploy env set prod DATABASE_URL="postgresql://user:pass@host/db"

# Apply changes immediately (rolling restart)
frankendeploy env set prod MAILER_DSN="smtp://..." --reload
```

Without `--reload`, changes take effect on the next deployment.

## Listing Variables

```bash
frankendeploy env list prod
```

Sensitive values (passwords, secrets, tokens) are automatically masked in the output.

## Getting a Single Variable

```bash
frankendeploy env get prod DATABASE_URL
```

## Removing Variables

```bash
frankendeploy env remove prod OLD_VARIABLE
```

## Bulk Operations

### Push a .env file

Push a local `.env` file to the server. Variables are merged with existing ones:

```bash
# Create a local .env.prod file
echo "APP_SECRET=my-secret" > .env.prod
echo "DATABASE_URL=postgresql://..." >> .env.prod

# Push to server
frankendeploy env push prod .env.prod

# Push and apply immediately
frankendeploy env push prod .env.prod --reload
```

### Pull from server

Download current environment variables to a local backup file:

```bash
frankendeploy env pull prod
# Creates .env.prod.backup
```

## Zero-Downtime Updates

The `--reload` flag performs a rolling restart:

1. Starts a new container with updated environment
2. Waits for health check to pass
3. Switches traffic to the new container
4. Removes the old container

This minimizes downtime to near-zero.

## Required Variables

For Symfony applications, you typically need:

| Variable | Description |
|----------|-------------|
| `APP_SECRET` | Symfony secret key (required) |
| `APP_ENV` | Environment (auto-set to `prod`) |
| `DATABASE_URL` | Database connection string |
| `MAILER_DSN` | Email transport (optional) |

## Security Notes

- Variables are stored in `/opt/frankendeploy/apps/<app>/shared/.env.local`
- The file is mounted read-only into containers
- Never commit `.env.prod.backup` files to git
- Use strong, unique secrets for production

## Workflow Example

```bash
# 1. Configure your production environment
frankendeploy env set prod APP_SECRET=$(openssl rand -hex 32)
frankendeploy env set prod DATABASE_URL="postgresql://user:pass@db.example.com/myapp"

# 2. Deploy your application
frankendeploy deploy prod --remote-build

# 3. Later, update a variable with zero downtime
frankendeploy env set prod FEATURE_FLAG=enabled --reload
```
