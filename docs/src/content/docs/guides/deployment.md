---
title: Deployment
description: Deploy your Symfony application to production
---

## Basic Deployment

```bash
frankendeploy deploy production
```

This command performs a complete deployment:
1. Builds Docker image locally
2. Transfers image to server
3. Stops old container
4. Starts new container
5. Runs health checks
6. Updates Caddy configuration
7. Cleans up old releases

## Deployment Options

### Custom Tag
```bash
frankendeploy deploy production --tag v1.2.3
```

By default, tags are timestamps like `20240115-143052`.

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

### Skip Build
If you've already built the image:
```bash
frankendeploy deploy production --no-build
```

### Force Deploy
Skip health check failures:
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

Create a health endpoint in your Symfony app:

```php
// src/Controller/HealthController.php
#[Route('/health')]
public function health(): Response
{
    return new Response('OK');
}
```

If the health check fails, FrankenDeploy automatically rolls back.

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
3. Caddy switches traffic to new container
4. Old container is stopped

If anything fails, traffic stays on the old container.

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
