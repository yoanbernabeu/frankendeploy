---
title: Local Development
description: Use FrankenDeploy for local Symfony development
---

## Development Environment

FrankenDeploy provides a Docker-based development environment that mirrors your production setup.

## Starting the Environment

```bash
frankendeploy dev up
```

This starts:
- Your Symfony application on **http://localhost:8000**
- Database (PostgreSQL or MySQL if configured)
- MailHog (if Symfony Mailer is detected) on **http://localhost:8025**
- RabbitMQ (if Symfony Messenger is detected) on **http://localhost:15672**

## Development Commands

### Start with Build
If you've made changes to the Dockerfile:
```bash
frankendeploy dev up --build
```

### View Logs
```bash
frankendeploy dev logs
frankendeploy dev logs -f  # Follow mode
```

### Stop Environment
```bash
frankendeploy dev down
```

### Restart
```bash
frankendeploy dev restart
```

## Volume Mounts

Your source code is mounted into the container, so changes are reflected immediately.

The following directories are excluded from mounts (for performance):
- `vendor/` - Installed fresh in container
- `node_modules/` - Installed fresh in container
- `var/` - Cache and logs

## Running Symfony Commands

Use Docker Compose to run commands in the container:

```bash
# Symfony console
docker compose exec app php bin/console cache:clear

# Composer
docker compose exec app composer require some/package

# Run tests
docker compose exec app php bin/phpunit
```

## Database Access

### PostgreSQL
```bash
# Connect via psql
docker compose exec database psql -U app -d app

# Connection details for your app
DATABASE_URL=postgresql://app:app@database:5432/app
```

### MySQL
```bash
# Connect via mysql
docker compose exec database mysql -u app -papp app

# Connection details
DATABASE_URL=mysql://app:app@database:3306/app
```

## Debugging with Xdebug

To enable Xdebug, add to your `frankendeploy.yaml`:

```yaml
php:
  extensions:
    - xdebug
  ini_values:
    - "xdebug.mode=debug"
    - "xdebug.client_host=host.docker.internal"
```

Then rebuild:
```bash
frankendeploy build
frankendeploy dev up --build
```

## Email Testing

If Symfony Mailer is detected, MailHog is automatically included.

- SMTP: `mailhog:1025`
- Web UI: http://localhost:8025

Add to your `.env.local`:
```
MAILER_DSN=smtp://mailhog:1025
```

## Hot Reload for Assets

If you're using Webpack Encore or Vite, run the dev server separately:

```bash
# In another terminal
npm run dev
# or
yarn dev
```

For AssetMapper, changes are reflected automatically.
