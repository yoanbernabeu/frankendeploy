---
title: frankendeploy.yaml
description: Project configuration reference
---

## Overview

The `frankendeploy.yaml` file in your project root configures how FrankenDeploy builds and deploys your application.

## Complete Reference

```yaml
# Application name (required)
# Used for Docker images, container names, directories
# Must be lowercase, alphanumeric with hyphens
name: my-app

# PHP Configuration
php:
  # PHP version: 8.1, 8.2, 8.3, or 8.4
  version: "8.3"

  # PHP extensions to install
  extensions:
    - pdo_pgsql
    - intl
    - opcache
    - redis
    - amqp

  # Custom php.ini values
  ini_values:
    - "memory_limit=256M"
    - "upload_max_filesize=50M"
    - "post_max_size=50M"

# Database Configuration (optional)
database:
  # Driver: pgsql, mysql, or sqlite
  driver: pgsql

  # Database version for Docker image
  version: "16"

  # Path: SQLite database file path (only for sqlite driver)
  # path: var/data.db

  # Managed: if true (default for pgsql/mysql), FrankenDeploy creates a DB container
  # If false, expects external DATABASE_URL in .env.local
  # Note: SQLite does NOT support managed mode (file-based database)
  managed: true

# Symfony Messenger Workers (optional)
messenger:
  # Enable dedicated worker container
  enabled: true

  # Number of worker processes
  workers: 2

  # Transports to consume
  transports:
    - async

# Dockerfile Customization (optional)
dockerfile:
  # Additional system packages
  extra_packages:
    - imagemagick
    - ffmpeg

  # Additional Dockerfile commands
  extra_commands:
    - "RUN pecl install imagick && docker-php-ext-enable imagick"

# Asset Build Configuration (optional)
assets:
  # Build tool: npm, yarn, pnpm, or assetmapper
  build_tool: npm

  # Command to build assets
  build_command: "npm run build"

  # Output directory (relative to project root)
  output_dir: "public/build"

# Deployment Configuration
deploy:
  # Domain for HTTPS (required for production)
  domain: my-app.com

  # Health check endpoint (default: /)
  healthcheck_path: /health

  # Number of releases to keep (default: 5)
  keep_releases: 5

  # Files shared between releases
  shared_files:
    - .env.local

  # Directories shared between releases
  shared_dirs:
    - var/log
    - var/sessions
    - public/uploads

  # Deployment hooks
  hooks:
    # Commands run before switching traffic
    pre_deploy:
      - php bin/console doctrine:migrations:migrate --no-interaction

    # Commands run after successful deployment
    post_deploy:
      - php bin/console cache:warmup

# Environment Variables
env:
  # Development environment
  dev:
    APP_DEBUG: "1"
    MAILER_DSN: "smtp://mailhog:1025"

  # Production environment
  prod:
    APP_DEBUG: "0"
    TRUSTED_PROXIES: "127.0.0.1,REMOTE_ADDR"
```

## Field Details

### `name` (required)

```yaml
name: my-app
```

- Must be unique per server
- Lowercase letters, numbers, and hyphens only
- Used as Docker image name and container name

### `php.version`

Supported versions:
- `8.1`
- `8.2`
- `8.3`
- `8.4`

### `php.extensions`

Common extensions:
- `pdo_pgsql` - PostgreSQL
- `pdo_mysql` - MySQL
- `intl` - Internationalization
- `opcache` - Performance
- `redis` - Redis cache
- `amqp` - RabbitMQ
- `gd` - Image processing
- `imagick` - ImageMagick
- `xdebug` - Debugging (dev only)

### `database.driver`

| Driver | Docker Image |
|--------|-------------|
| `pgsql` | postgres:VERSION-alpine |
| `mysql` | mysql:VERSION |
| `sqlite` | No container needed |

### `database.path`

**SQLite only.** The file path for the SQLite database (relative to project root).

```yaml
database:
  driver: sqlite
  path: var/data.db
```

When using SQLite, FrankenDeploy automatically adds the database directory (e.g., `var`) to `shared_dirs` to ensure data persistence across deployments.

### `database.managed`

Controls how the database is provisioned in production:

| Value | Behavior |
|-------|----------|
| `true` (default for pgsql/mysql) | FrankenDeploy creates a Docker container for the DB |
| `false` | Use external database, set `DATABASE_URL` in `.env.local` |

:::caution[SQLite Limitation]
SQLite does **not** support `managed: true`. SQLite is a file-based database and cannot run as a container. If you set `managed: true` with SQLite, validation will fail with an error.

For SQLite persistence in production, ensure the database directory is in `shared_dirs`.
:::

### `messenger`

Configures Symfony Messenger worker containers:

```yaml
messenger:
  enabled: true     # Deploy a dedicated worker container
  workers: 2        # Number of worker processes
  transports:       # Transports to consume
    - async
    - high_priority
```

When enabled, FrankenDeploy deploys a separate container (`<app>-worker`) running `messenger:consume`.

### `dockerfile`

Customize the generated Dockerfile:

```yaml
dockerfile:
  extra_packages:     # APT packages to install
    - imagemagick
    - ffmpeg
  extra_commands:     # Raw Dockerfile instructions
    - "RUN pecl install redis && docker-php-ext-enable redis"
```

### `assets.build_tool`

| Tool | Detection |
|------|-----------|
| `npm` | package-lock.json |
| `yarn` | yarn.lock |
| `pnpm` | pnpm-lock.yaml |
| `assetmapper` | importmap.php |

### `deploy.hooks`

Hooks run inside the container. Available commands:
- Symfony console: `php bin/console ...`
- Composer: `composer ...`
- Any installed binary

**Auto-fill:** When running `frankendeploy init`, hooks are automatically populated based on detected features:
- If Doctrine is detected: `php bin/console doctrine:migrations:migrate --no-interaction` in `pre_deploy`
- If Symfony: `php bin/console cache:warmup` in `post_deploy`

### `env`

Environment variables are passed to Docker. For secrets, use:
- Server environment variables
- Docker secrets
- External secrets manager

## Validation

Run to validate your configuration:

```bash
frankendeploy build
```

Errors are displayed with details on how to fix them.
