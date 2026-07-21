---
title: Deployment
description: Deploy your Symfony application to production
---

## Basic Deployment

```bash
frankendeploy deploy production
```

This command performs a complete blue-green deployment:
1. Generates missing Docker artifacts (`Dockerfile`, `docker-entrypoint.sh`, `.dockerignore`) — no need to run `build` first, and customized files are never overwritten
2. Builds the Docker image (locally, or on the server with remote build)
3. Transfers it to the server
4. Sets up the managed database container if configured (`DATABASE_URL` is injected automatically)
5. Starts the **new** container alongside the old one and runs pre-deploy hooks
6. Runs health checks on the new container
7. Swaps traffic to the new container (zero downtime), then stops the old one
8. Updates Caddy configuration and cleans up old releases

If any step fails before the swap, the old container keeps serving traffic untouched.

## Deployment Options

### Custom Tag
```bash
frankendeploy deploy production --tag v1.2.3
```

By default, tags are timestamps like `20240115-143052`.

### Cross-Architecture Detection

FrankenDeploy automatically detects architecture mismatches between your local machine and the server. When deploying from Apple Silicon (ARM) to an x86_64 VPS, you'll see:

```
⚠️  Architecture mismatch detected:
   Local:  arm64 (Apple Silicon)
   Server: x86_64

   Local builds will not run on this server.
```

In **interactive mode**, FrankenDeploy prompts you to enable remote build and saves your preference for future deployments.

In **CI/CD mode** (`--yes`), you must explicitly use `--remote-build` or pre-configure the server:

```bash
# Configure server for remote builds
frankendeploy server set production remote_build true

# Or use the flag
frankendeploy deploy production --remote-build --yes
```

### Remote Build (Recommended for Apple Silicon)
Build the Docker image directly on the server instead of locally:
```bash
frankendeploy deploy production --remote-build
```

This is **recommended** when:
- Your local machine has a different architecture (Apple Silicon → x86 VPS)
- Local builds are slow due to emulation
- You want faster transfers (source code vs Docker image)

How it works:
1. Transfers source code via `rsync` (fast, excludes node_modules/vendor)
2. Builds Docker image on the VPS
3. Deploys normally

### Force Local Build
If remote build is configured but you want to build locally anyway:
```bash
frankendeploy deploy production --no-remote-build
```

### Skip Build
If you've already built the image:
```bash
frankendeploy deploy production --no-build
```

### Skip Individual Checks
```bash
# Skip the pre-flight environment variables check
frankendeploy deploy production --skip-env-check

# Skip the health check entirely (traffic switches unverified)
frankendeploy deploy production --skip-healthcheck
```

### Force Deploy
`--force` skips the env pre-flight and continues even when pre-deploy hooks (e.g. migrations) or the health check fail — use with care:
```bash
frankendeploy deploy production --force
```

## Health Checks

FrankenDeploy verifies your application is healthy before switching traffic.

Configure in `frankendeploy.yaml`:
```yaml
deploy:
  healthcheck_path: /health
```

The default path is `/`. For **API Platform** projects, `init` detects the framework and sets the path to `/api` automatically — a pure API returns 404 on `/`, which would fail every health check. The same path is also used by Caddy's active health checks on the running container **and by the Docker `HEALTHCHECK` baked into the generated image**, so `docker ps` health status reflects the application actually answering, not just the web server process being up.

The check window is generous by default (90 seconds — a cold Symfony container needs time for opcache warmup and database wait) and tunable:

```yaml
deploy:
  healthcheck_timeout: 90    # overall window in seconds
  healthcheck_retries: 30    # max attempts
  healthcheck_interval: 3    # seconds between attempts
```

When the health check fails, FrankenDeploy prints the **last 50 log lines of the failing container** before removing it, so you immediately see the real cause (missing env variable, failed migration, PHP fatal…).

Create a health endpoint in your Symfony app:

```php
// src/Controller/HealthController.php
#[Route('/health')]
public function health(): Response
{
    return new Response('OK');
}
```

If the health check fails, the new container is removed and the old one keeps serving traffic — a failed deployment never takes your site down.

## Deployment Hooks

Run commands before and after deployment:

```yaml
deploy:
  hooks:
    pre_deploy:
      - php bin/console doctrine:migrations:migrate --no-interaction
      - php bin/console messenger:stop-workers
    post_deploy:
      - php bin/console cache:warmup
```

**Pre-deploy hooks** run in the new container before traffic is switched.
**Post-deploy hooks** run after successful deployment.

## Database Migration Warning

FrankenDeploy automatically detects when your project has Doctrine entities but no migration files. This can happen when you forget to generate migrations after creating entities.

When this situation is detected (entities in `src/Entity/` but no files in `migrations/`), FrankenDeploy displays a warning:

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

This warning only appears once per application. Once you add migrations and redeploy, the warning is automatically cleared.

**Note:** This check only runs when you have a migration hook configured in your `pre_deploy` hooks.

## Release Management

FrankenDeploy keeps multiple releases for instant rollback:

```yaml
deploy:
  keep_releases: 5
```

Releases are stored in `/opt/frankendeploy/apps/your-app/releases/`.

View releases:
```bash
frankendeploy app status production
```

## Environment Variables

Set production environment variables in `frankendeploy.yaml`:

```yaml
env:
  prod:
    APP_ENV: prod
    APP_DEBUG: "0"
    DATABASE_URL: "${DATABASE_URL}"
```

For secrets, set them directly on the server or use a secrets manager.

## Shared Files and Directories

Files and directories that persist between releases:

```yaml
deploy:
  shared_files:
    - .env.local
  shared_dirs:
    - var/log
    - var/sessions
    - public/uploads
```

## Zero-Downtime Deployment

FrankenDeploy ensures zero downtime:

1. New container starts alongside old one
2. Health checks verify new container
3. Traffic switches to the new container via a rename-based swap (Docker's embedded DNS follows the container name, so no request is dropped)
4. Old container is stopped

If anything fails — including the swap itself — traffic stays on the old container.

## Managed Database

With `database.managed: true` (the default for PostgreSQL/MySQL), each deployment ensures the database container is running and injects the generated `DATABASE_URL` into your application automatically. Credentials are created once and persist across deployments — you never have to set `DATABASE_URL` yourself.

## Monitoring Deployments

### View Logs During Deploy
Use verbose mode:
```bash
frankendeploy deploy production --verbose
```

### Check Deployment Status
```bash
frankendeploy app status production
```

### View Application Logs
```bash
frankendeploy logs production
frankendeploy logs production -f  # Follow mode
```

## CI/CD Integration

FrankenDeploy provides environment variables and flags for seamless CI/CD integration.

### Environment Variables

| Variable | Description |
|----------|-------------|
| `FRANKENDEPLOY_SERVER` | Server name to use (alternative to argument) |
| `FRANKENDEPLOY_SSH_KEY` | SSH private key content (base64 or raw) |
| `FRANKENDEPLOY_KNOWN_HOSTS` | Known hosts file content |
| `FRANKENDEPLOY_SKIP_HOST_KEY_CHECK` | Skip host key verification (not recommended) |

### Non-Interactive Mode

Use `--yes` or `-y` to skip all confirmation prompts:

```bash
frankendeploy deploy production --yes
frankendeploy app remove production my-app --force --yes
```

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
        run: |
          curl -fsSL https://raw.githubusercontent.com/yoanbernabeu/frankendeploy/main/scripts/install.sh | sh

      - name: Deploy
        env:
          FRANKENDEPLOY_SERVER: production
          FRANKENDEPLOY_SSH_KEY: ${{ secrets.SSH_KEY }}
          FRANKENDEPLOY_KNOWN_HOSTS: ${{ secrets.KNOWN_HOSTS }}
        run: frankendeploy deploy --yes
```

### GitLab CI Example

```yaml
deploy:
  stage: deploy
  image: ubuntu:22.04
  script:
    - curl -fsSL https://raw.githubusercontent.com/yoanbernabeu/frankendeploy/main/scripts/install.sh | sh
    - frankendeploy deploy --yes
  variables:
    FRANKENDEPLOY_SERVER: production
    FRANKENDEPLOY_SSH_KEY: $SSH_PRIVATE_KEY
    FRANKENDEPLOY_KNOWN_HOSTS: $KNOWN_HOSTS
  only:
    - main
```
