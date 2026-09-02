---
title: Project Configuration
description: How frankendeploy init builds your frankendeploy.yaml, and how to adjust it
---

## The Configuration File

FrankenDeploy reads one file, `frankendeploy.yaml`, at the root of your project. `frankendeploy init` creates it from what it detects; you can then edit it by hand. It is versioned with your code and contains no secret.

The full list of fields is in the [frankendeploy.yaml reference](/frankendeploy/config/project/). This page explains where the values come from.

## A Typical File

```yaml
name: my-app

php:
  version: "8.3"
  extensions:
    - intl
    - opcache
    - zip
    - pdo_pgsql

database:
  driver: pgsql
  version: "16"
  managed: true

assets:
  build_tool: assetmapper

deploy:
  domain: my-app.com
  healthcheck_path: /api
  keep_releases: 5
  hooks:
    pre_deploy:
      - php bin/console doctrine:migrations:migrate --no-interaction --allow-no-migration
```

## Auto-detection

`frankendeploy init` scans the project and announces every inference:

| Feature | Detected from | Result |
|---------|---------------|--------|
| PHP version | `composer.json` `require.php` | `php.version`, floored at 8.2 (FrankenPHP requirement) |
| PHP extensions | `composer.json` `ext-*` and known packages, database driver, Messenger DSNs | `php.extensions` (always includes `intl`, `opcache`, `zip`) |
| Database | `DATABASE_URL` in `.env` and `.env.local` (`.env.local` wins) | `database.driver`, `database.version`, `database.path` for SQLite |
| Assets | `package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`, `importmap.php`, Vite/Webpack config | `assets.build_tool`, `assets.tailwind` |
| Messenger | `config/packages/messenger.yaml` | `messenger.enabled`, `messenger.transports` (plus `scheduler_default` with `symfony/scheduler`) |
| Mailer | `config/packages/mailer.yaml` | `mailer.enabled` (Mailpit in dev) |
| API Platform | `api-platform/*` in `composer.json` | `deploy.healthcheck_path: /api` |
| Doctrine | `config/packages/doctrine.yaml` | migration hook in `deploy.hooks.pre_deploy` |
| Worker mode | `symfony/runtime` >= 7.4 or `runtime/frankenphp-symfony` | `frankenphp.worker: true` |

When `.env` and `.env.local` disagree on the database driver, `init` warns and follows `.env.local`, as Symfony would at runtime.

## Init Options

```bash
# Set the production domain right away
frankendeploy init --domain my-app.com

# Use a custom application name (default: the directory name)
frankendeploy init --name my-custom-name

# Overwrite an existing frankendeploy.yaml
frankendeploy init --force
```

Without `--domain`, add it later under `deploy.domain`: the app deploys fine without one, it just is not publicly reachable.

## Adjusting the File

The most common edits after `init`:

- **Shared directories**: add `public/uploads` (or any directory that must survive a deploy) to `deploy.shared_dirs`. Setting the list replaces the default `var/log`, `var/sessions`.
- **Resource limits**: `deploy.memory_limit: 512m` and `deploy.cpu_limit: "1.5"` cap the app container.
- **Health check**: point `deploy.healthcheck_path` to a route that proves the app works (a database query, for instance), and widen the window with `healthcheck_timeout` if a cold start takes long.
- **Extra system packages or PHP extensions**: `dockerfile.extra_packages` and `php.extensions`.
- **Pin FrankenPHP**: `frankenphp_version: "1.4"` for reproducible builds.

Production **environment variables and secrets are not in this file**: they live on the server, managed with [`frankendeploy env`](/frankendeploy/guides/environment-variables/).

## Validation

The file is validated every time it is loaded, and unknown fields are rejected (a typo never fails silently). To validate without doing anything else:

```bash
frankendeploy build
```
