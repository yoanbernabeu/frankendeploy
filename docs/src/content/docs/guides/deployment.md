---
title: Deployment
description: Deploy your Symfony application to production
---

## Basic Deployment

```bash
frankendeploy deploy prod
```

One command, a complete blue-green deployment:

1. Pre-flight check of the environment variables the app needs on the server (`APP_SECRET`, `DATABASE_URL` for an external database)
2. Generates missing Docker artifacts (`Dockerfile`, `docker-entrypoint.sh`, `.dockerignore`, `Caddyfile` in worker mode). No need to run `build` first; customized files are never overwritten
3. Builds the Docker image, locally or on the server with remote build, and transfers it
4. Ensures the application's isolated Docker network exists
5. Starts the managed database container if configured, and injects `DATABASE_URL`
6. Starts the **new** container next to the old one, backs up the database, runs the `pre_deploy` hooks (migrations)
7. Runs the health check on the new container
8. Switches traffic to the new container (zero downtime), then stops the old one
9. Restarts the Messenger worker on the new image, runs the `post_deploy` hooks
10. Writes the Caddy configuration for the domain and reloads the proxy
11. Cleans up old releases, images and backups beyond `keep_releases`

If any step fails before the switch, the old container keeps serving traffic untouched. Before a first deploy, run [`frankendeploy doctor prod`](/frankendeploy/guides/doctor/).

## Deployment Options

### Custom Tag
```bash
frankendeploy deploy prod --tag v1.2.3
```

By default, tags are timestamps like `20260902-143052`. The tag names the release and the Docker image.

### Cross-Architecture Detection

FrankenDeploy compares the architecture of your machine and of the server. Deploying from Apple Silicon (arm64) to an x86_64 VPS, you see:

```
⚠️  Architecture mismatch detected:
   Local:  arm64 (Apple Silicon)
   Server: x86_64

   Local builds will not run on this server.
```

In **interactive mode**, FrankenDeploy offers to enable remote build and saves the preference on the server entry (`remote_build: true` in the [global config](/frankendeploy/config/global/)).

In **CI/CD mode** (`--yes`), either pass `--remote-build` or configure the server once:

```bash
frankendeploy server set prod remote_build true
```

### Remote Build (recommended for Apple Silicon)
Build the Docker image on the server instead of locally:
```bash
frankendeploy deploy prod --remote-build
```

How it works:
1. The source code is transferred over the existing SSH connection (pure-Go SFTP: no rsync or scp needed, works on Windows; `.git`, `node_modules`, `vendor`, `var` and `.env.local` are excluded)
2. The image is built on the VPS, with Docker's layer cache, so the second build is much faster than the first
3. The deploy continues normally

Recommended when your machine has a different architecture than the server, when local builds are slow, or when your upload bandwidth is small (source code is much smaller than an image).

### Force Local Build
If remote build is configured on the server but you want a local build this time:
```bash
frankendeploy deploy prod --no-remote-build
```

### Skip Build
If the image with this tag already exists locally:
```bash
frankendeploy deploy prod --no-build --tag v1.2.3
```

### Skip Individual Checks
```bash
# Skip the pre-flight environment variables check
frankendeploy deploy prod --skip-env-check

# Skip the health check entirely (traffic switches unverified)
frankendeploy deploy prod --skip-healthcheck
```

### Force Deploy
`--force` skips the env pre-flight and continues even when the database backup, the `pre_deploy` hooks (migrations) or the health check fail. Use with care:
```bash
frankendeploy deploy prod --force
```

## Health Checks

FrankenDeploy verifies that the new container answers before switching traffic to it.

```yaml
deploy:
  healthcheck_path: /health
```

The default path is `/`. For **API Platform** projects, `init` sets it to `/api` automatically: a pure API returns 404 on `/`, which would fail every health check. The same path is used by the Docker `HEALTHCHECK` baked into the image, so `docker ps` health status reflects the application actually answering, not just the web server process being up.

The reverse proxy deliberately runs **no active health check** on the live container: with a single upstream there is nothing to fail over to, and a probe timing out under load would turn a slow application into a 503 for every visitor. Per-request timeouts apply instead (5 s to connect, 60 s for the response headers): a container that accepts connections but never answers costs the affected visitor a 504, not an endless wait.

The check window is generous by default (90 seconds: a cold Symfony container needs time for opcache warmup and database wait) and tunable:

```yaml
deploy:
  healthcheck_timeout: 90    # overall window in seconds
  healthcheck_retries: 30    # max attempts
  healthcheck_interval: 3    # seconds between attempts
```

When the health check fails, FrankenDeploy prints the **last 50 log lines of the failing container** before removing it, so you immediately see the real cause (missing variable, failed migration, PHP fatal…).

A good health endpoint proves the app works, not only that PHP runs:

```php
// src/Controller/HealthController.php
#[Route('/health')]
public function health(Connection $connection): Response
{
    $connection->executeQuery('SELECT 1');

    return new Response('OK');
}
```

## Deployment Hooks

```yaml
deploy:
  hooks:
    pre_deploy:
      - php bin/console doctrine:migrations:migrate --no-interaction --allow-no-migration
    post_deploy:
      - php bin/console cache:pool:clear cache.app
```

**`pre_deploy`** hooks run in the new container, before traffic is switched, while the old version still serves requests. A failure removes the new container and aborts the deploy (unless `--force`).

**`post_deploy`** hooks run in the live container after the switch. A failure only warns.

No `cache:warmup` hook is needed: the Symfony cache is warmed when the image is built.

## Database Migration Warning

FrankenDeploy detects when the project has Doctrine entities but no migration files, a classic when you forget `make:migration` after creating entities:

```
⚠️  Warning: No database migrations found but entities exist!

   Entities found: 5 files in src/Entity/
   Migrations:     0 files in migrations/

   This may cause 'no such table' errors at runtime.

   To fix this, run locally:
      php bin/console make:migration
      php bin/console doctrine:migrations:migrate
      git add migrations/
      git commit -m "Add database migrations"

   Then redeploy your application.
```

The warning appears once per application and clears itself once migrations exist. It only runs when a migration hook is configured in `pre_deploy`.

## Automatic Database Backup Before Migrations

With a **managed database** (`database.managed: true`), FrankenDeploy dumps the database before any `pre_deploy` hook containing `doctrine:migrations:migrate`:

- The gzipped dump is stored on the server in `/opt/frankendeploy/apps/<app>/shared/backups/` (permissions `600`)
- Retention follows `deploy.keep_releases` (default: 5 backups)
- A failed backup **aborts the deploy**. `--force` deploys without this safety net (not recommended)

Why this matters: migrations run **before** the traffic switch, while the old code still serves requests. If the health check fails after a successful migration, the containers are rolled back but **the database schema is not**: the previous version then runs on the new schema. When that happens, FrankenDeploy says so and points to the backup:

```
⚠️  The database was already migrated during this deploy. Rolling back the code may not be enough...
⚠️  Database backup taken before the migration: /opt/frankendeploy/apps/my-app/shared/backups/pgsql-20260902-120000.sql.gz
```

Restore example (PostgreSQL):

```bash
gunzip -c backups/pgsql-<tag>.sql.gz | docker exec -i <app>-db psql -U <user> <db>
```

**Best practice:** write backward-compatible migrations (expand/contract: add columns before using them, drop them one release later). A code rollback then stays safe even after a migration.

For an **external database**, no automatic backup is possible: back it up yourself before deploying migrations.

## Messenger Workers

With `messenger.enabled: true`, each deploy starts one `<app>-worker` container from the same image, running:

```
php bin/console messenger:consume <transports> --time-limit=3600 --memory-limit=256M
```

The worker is stopped and recreated after the traffic switch, so it always runs the same code as the app, and `rollback` does the same. `--time-limit` and `--memory-limit` make it restart cleanly (Docker's `restart: unless-stopped` brings it back), which is the recommended way to run long-lived PHP workers. A failure to start the worker warns but does not fail the deploy.

```bash
frankendeploy logs prod --service worker      # worker logs
frankendeploy logs prod --service all -f      # app and worker
```

## Release Management

```yaml
deploy:
  keep_releases: 5
```

Releases are stored in `/opt/frankendeploy/apps/<app>/releases/<tag>/`, with `current` pointing to the live one. `keep_releases` also drives **disk usage**: after each deploy, FrankenDeploy removes the Docker images whose tag left the retention window (never an image in use by a container), and the database backups beyond the same count. With remote builds, dangling intermediate layers are pruned after each build. Rollback targets and disk retention therefore always match.

```bash
frankendeploy app status prod
```

## Environment Variables

Production variables live **on the server**, per application, in `/opt/frankendeploy/apps/<app>/shared/.env.local`, and are managed with `frankendeploy env`:

```bash
openssl rand -hex 32 | frankendeploy env set prod APP_SECRET --from-stdin
frankendeploy env set prod MAILER_DSN="smtp://..." --reload
```

FrankenDeploy itself sets `SERVER_NAME`, `APP_ENV=prod`, `APP_DEBUG=0` and, with a managed database, `DATABASE_URL`. The `env.prod` section of `frankendeploy.yaml` only feeds the generated `compose.prod.yaml` and is **not** read by `deploy`. See [Environment Variables](/frankendeploy/guides/environment-variables/).

## Shared Files and Directories

Paths that persist between releases, mounted into every container from `/opt/frankendeploy/apps/<app>/shared/`:

```yaml
deploy:
  shared_files:
    - .env.local          # mounted read-only
  shared_dirs:
    - var/log
    - var/sessions
    - public/uploads
```

## Zero-Downtime Deployment

1. The new container starts next to the old one, under a temporary name
2. Migrations run and the health check verifies the new container
3. Traffic switches through a rename-based swap: the old container is renamed away while still running, the new one takes the app name, and Docker's embedded DNS follows the name, so Caddy never sees a missing upstream
4. The old container is stopped

If anything fails, including the swap itself, traffic stays on the old container.

## Network Isolation

Each application runs on its own Docker network, `frankendeploy-<app>`, with its worker and its managed database. Caddy is attached to every app network so it can reach `<app>:8080` by name; nothing else is. Two applications deployed on the same VPS cannot reach each other's containers, so a compromised app cannot talk to another app's database.

The network is created by the first deploy. An application deployed before per-app networks existed (FrankenDeploy < 0.15) is migrated transparently on its next deploy: the database container joins the app network, the new app container starts on it, and once the old container is stopped the database leaves the shared network. Every step checks the current state before acting, so a deploy interrupted in the middle completes on the next run. `frankendeploy doctor` reports the isolation status of the app.

## Managed Database

With `database.managed: true` (the default for PostgreSQL, MySQL and MariaDB), the first deploy creates the `<app>-db` container with random credentials and a persistent volume, and every deploy makes sure it runs and injects `DATABASE_URL` into the app. Credentials are saved on the server (`shared/.db_credentials`, permissions `600`) and reused: you never set `DATABASE_URL` yourself, and the database survives deploys, rollbacks and reboots.

## Monitoring Deployments

```bash
frankendeploy deploy prod --verbose       # every command run on the server
frankendeploy app status prod             # container status, releases
frankendeploy logs prod -f                # follow the app logs
frankendeploy logs prod --since 10m       # last 10 minutes
frankendeploy logs prod --tail 500        # last 500 lines
```

## CI/CD Integration

### Environment Variables

| Variable | Description |
|----------|-------------|
| `FRANKENDEPLOY_SERVER` | Server name to use (alternative to the argument) |
| `FRANKENDEPLOY_SSH_KEY` | Content of the SSH private key (raw PEM, not base64). Encrypted keys cannot be used here: use an unencrypted deploy key |
| `FRANKENDEPLOY_KNOWN_HOSTS` | Content of a `known_hosts` file with the server's host key |
| `FRANKENDEPLOY_SKIP_HOST_KEY_CHECK` | Skip host key verification (not recommended) |

The server itself must be declared on the runner: run `frankendeploy server add prod user@host --skip-test` in the job, as in the examples below.

### Non-Interactive Mode

`--yes` (`-y`) answers every prompt with its default and never waits for input:

```bash
frankendeploy deploy prod --yes
frankendeploy app remove prod my-app --force --yes
```

`frankendeploy doctor prod` exits with code 1 on a blocking problem, which makes it a good gate before `deploy`.

### GitHub Actions Example

```yaml
name: Deploy

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install FrankenDeploy
        run: curl -fsSL https://raw.githubusercontent.com/yoanbernabeu/frankendeploy/main/scripts/install.sh | sh

      - name: Deploy
        env:
          FRANKENDEPLOY_SERVER: prod
          FRANKENDEPLOY_SSH_KEY: ${{ secrets.SSH_KEY }}
          FRANKENDEPLOY_KNOWN_HOSTS: ${{ secrets.KNOWN_HOSTS }}
        run: |
          frankendeploy server add prod ${{ secrets.SSH_TARGET }} --skip-test
          frankendeploy doctor prod
          frankendeploy deploy --yes
```

`SSH_TARGET` is `user@host`. The image is built on the runner (x86_64, like most VPS), so no remote build is needed.

### GitLab CI Example

```yaml
deploy:
  stage: deploy
  image: ubuntu:22.04
  before_script:
    - apt-get update && apt-get install -y curl ca-certificates
    - curl -fsSL https://raw.githubusercontent.com/yoanbernabeu/frankendeploy/main/scripts/install.sh | sh
  script:
    - frankendeploy server add prod $SSH_TARGET --skip-test
    - frankendeploy doctor prod
    - frankendeploy deploy --yes
  variables:
    FRANKENDEPLOY_SERVER: prod
    FRANKENDEPLOY_SSH_KEY: $SSH_PRIVATE_KEY
    FRANKENDEPLOY_KNOWN_HOSTS: $KNOWN_HOSTS
  only:
    - main
```
