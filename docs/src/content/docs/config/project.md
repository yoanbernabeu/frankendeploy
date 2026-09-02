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
  # PHP version: 8.2, 8.3, or 8.4 (FrankenPHP requires PHP >= 8.2)
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

# FrankenPHP Runtime Options (optional)
frankenphp:
  # Worker mode: boots the Symfony kernel once and keeps it in memory
  # between requests (significant latency improvement).
  # Requires symfony/runtime >= 7.4 (native worker runner) or the
  # runtime/frankenphp-symfony composer package.
  # Auto-enabled by 'frankendeploy init' when either is detected.
  worker: true

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

  # Health check endpoint (default: /, or /api when API Platform is detected)
  healthcheck_path: /health

  # Health check tuning (optional, 0 or omitted = defaults)
  healthcheck_timeout: 90    # overall window in seconds (default: 90)
  healthcheck_retries: 30    # max attempts (default: 30)
  healthcheck_interval: 3    # seconds between attempts (default: 3)

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
- `8.2`
- `8.3`
- `8.4`

FrankenPHP requires PHP >= 8.2. If your `composer.json` allows older versions (e.g. `>=8.1`), `init` floors the detected version at 8.2 and warns you.

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

### `frankenphp.worker`

FrankenPHP worker mode boots the Symfony kernel **once** and keeps it in memory between requests, removing the per-request bootstrap cost.

```yaml
frankenphp:
  worker: true
```

Requirements and constraints:

- A FrankenPHP worker runtime must be installed: `symfony/runtime` >= 7.4 ships the worker runner natively (nothing to add on a recent Symfony), older apps can use the `runtime/frankenphp-symfony` package. `frankendeploy build` fails with a clear message if neither is present.
- The generated Caddyfile only forces `APP_RUNTIME` for `runtime/frankenphp-symfony`; with the native runtime, Symfony selects the worker runner itself.
- **The application must be stateless**: any static property or service state survives between requests. Symfony services are reset automatically, but hand-written static caches are not.
- Memory leaks surface as automatic worker restarts (FrankenPHP restarts workers hitting their memory limit).
- `frankendeploy init` enables it automatically (with a warning) when a worker runtime is detected. Opt out with `worker: false`.
- Worker mode applies to **production only** — the dev environment keeps classic mode for easier debugging.

When enabled, `frankendeploy build` generates a `Caddyfile` at the project root that is baked into the production image.

### `database.driver`

| Driver | Docker Image |
|--------|-------------|
| `pgsql` | postgres:VERSION-alpine |
| `mysql` | mysql:VERSION |
| `mariadb` | mariadb:VERSION |
| `sqlite` | No container needed |

MariaDB is auto-detected from `DATABASE_URL` (a `mariadb://` scheme or a `serverVersion` ending in `-MariaDB`). The generated `DATABASE_URL` uses the `mysql://` scheme with the `-MariaDB` serverVersion suffix, as Doctrine expects.

The scanner reads `DATABASE_URL` from `.env` **and** `.env.local` (the latter wins, matching Symfony's runtime behavior), and warns when the two files disagree on the driver.

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

`frankendeploy init` fills `transports` from the transports actually configured in `config/packages/messenger.yaml` (resolving `%env()%` DSNs through `.env`/`.env.local`). The failure transport and `sync://` transports are excluded. With `symfony/scheduler` installed, `scheduler_default` is added so scheduled tasks actually run in production. An `amqp://` or `redis://` transport DSN also adds the matching PHP extension, and `pcntl` is always added with Messenger for graceful worker shutdown — every inference is announced at `init`.

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

### `assets.tailwind`

With `symfonycasts/tailwind-bundle` (AssetMapper), `tailwind:build --minify` must run **before** `asset-map:compile` — otherwise the site deploys without CSS. The scanner detects the bundle and sets `tailwind: true` automatically (announced at `init`).

```yaml
assets:
  build_tool: assetmapper
  tailwind: true
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
