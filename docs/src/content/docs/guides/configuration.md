---
title: Project Configuration
description: Configure frankendeploy.yaml for your project
---

## Configuration File

FrankenDeploy uses `frankendeploy.yaml` in your project root. This file is created by `frankendeploy init` and can be customized.

## Full Example

```yaml
name: my-app

php:
  version: "8.3"
  extensions:
    - pdo_pgsql
    - intl
    - opcache
    - redis
  ini_values:
    - "memory_limit=256M"
    - "upload_max_filesize=50M"

database:
  driver: pgsql
  version: "16"

assets:
  build_tool: npm
  build_command: "npm run build"
  output_dir: "public/build"

deploy:
  domain: my-app.com
  healthcheck_path: /health
  keep_releases: 5
  shared_files:
    - .env.local
  shared_dirs:
    - var/log
    - var/sessions
  hooks:
    pre_deploy:
      - php bin/console doctrine:migrations:migrate --no-interaction
    post_deploy:
      - php bin/console cache:warmup

env:
  dev:
    APP_DEBUG: "1"
  prod:
    APP_DEBUG: "0"
```

## Configuration Options

### `name`

Your application name. Used for Docker images, container names, and directory structure.

```yaml
name: my-symfony-app
```

Must be lowercase, alphanumeric with hyphens only.

### `php`

PHP configuration for your application.

```yaml
php:
  version: "8.3"      # PHP version (8.1, 8.2, 8.3, 8.4)
  extensions:         # PHP extensions to install
    - pdo_pgsql
    - intl
  ini_values:         # Custom php.ini settings
    - "memory_limit=256M"
```

### `database`

Database configuration for Docker Compose.

```yaml
database:
  driver: pgsql       # pgsql, mysql, or sqlite
  version: "16"       # Database version
```

#### SQLite Configuration

SQLite is a file-based database and is handled differently from PostgreSQL and MySQL:

```yaml
database:
  driver: sqlite
  version: "3"
  path: var/data.db   # Automatically detected from DATABASE_URL
```

Key differences for SQLite:
- **No `managed` option**: SQLite cannot run as a container, so `managed: true` is not allowed
- **Automatic shared_dirs**: The SQLite database directory is automatically added to `shared_dirs` for persistence
- **Path detection**: The file path is extracted from your `DATABASE_URL` in `.env`

Example `.env` for SQLite:
```bash
DATABASE_URL="sqlite:///%kernel.project_dir%/var/data.db"
```

### `assets`

Frontend asset build configuration.

```yaml
assets:
  build_tool: npm        # npm, yarn, pnpm, or assetmapper
  build_command: "npm run build"
  output_dir: "public/build"
```

For Symfony AssetMapper, set:
```yaml
assets:
  build_tool: assetmapper
```

### `deploy`

Deployment configuration.

```yaml
deploy:
  domain: example.com           # Domain for HTTPS (Caddy config)
  healthcheck_path: /health     # Health check endpoint
  keep_releases: 5              # Number of releases to keep
  shared_files:                 # Files shared between releases
    - .env.local
  shared_dirs:                  # Directories shared between releases
    - var/log
    - var/sessions
  hooks:
    pre_deploy:                 # Commands before switching traffic
      - php bin/console doctrine:migrations:migrate --no-interaction
    post_deploy:                # Commands after deployment
      - php bin/console cache:warmup
```

### `env`

Environment variables for different environments.

```yaml
env:
  dev:
    APP_DEBUG: "1"
    MAILER_DSN: "smtp://mailhog:1025"
  prod:
    APP_DEBUG: "0"
    TRUSTED_PROXIES: "127.0.0.1,REMOTE_ADDR"
```

## Auto-detection

When you run `frankendeploy init`, it automatically detects:

| Feature | Detection Method |
|---------|------------------|
| PHP version | `composer.json` require.php constraint |
| PHP extensions | `composer.json` ext-* requirements |
| Database | `doctrine.yaml`, `.env` DATABASE_URL |
| Assets | `package.json`, `vite.config.js`, `importmap.php` |

## Validation

FrankenDeploy validates your configuration. Run:

```bash
frankendeploy build
```

If there are issues, you'll see validation errors with details on how to fix them.
