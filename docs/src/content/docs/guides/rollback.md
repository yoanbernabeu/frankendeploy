---
title: Rollback & Recovery
description: How to rollback to a previous release
---

## Quick Rollback

Roll back to the previous release:

```bash
frankendeploy rollback production
```

This switches traffic back to the previous release with the **same guarantees as a deployment**:
- The rollback container gets the same shared directories, environment and managed `DATABASE_URL` as a regular deploy
- A health check runs **before** the swap — if the old release is unhealthy, the rollback aborts and the current version keeps running
- The swap itself is zero-downtime (same rename-based mechanism as deploy)
- Messenger workers are rolled back to the same release

## Rollback to Specific Release

List available releases:
```bash
frankendeploy app status production
```

Output:
```
Application: my-app
Server:      production

Status:      running
Release:     20240115-143052

Recent releases:
  * 20240115-143052
    20240115-120000
    20240114-180000
    20240114-150000
    20240113-100000
```

Rollback to a specific release:
```bash
frankendeploy rollback production 20240114-180000
```

## Automatic Rollback

During a deployment, FrankenDeploy protects the running version if:

1. **Health check fails** - The new container doesn't respond correctly
2. **Container won't start** - Docker fails to start the container
3. **Pre-deploy hooks fail** - Migration or other commands fail

In all these cases the new container is removed and the **old container keeps serving traffic untouched** — there is nothing to restore because traffic never left the working version:

```
⚠️  Health check failed, rolling back...
```

## Managing Releases

### Keep More Releases
```yaml
deploy:
  keep_releases: 10
```

### Cleanup Old Releases
Old releases are automatically cleaned after each deployment. Only the configured number of releases are kept (see `deploy.keep_releases` in `frankendeploy.yaml`).

To manually clean releases, connect to your server via SSH:
```bash
ssh user@your-server
rm -rf /opt/frankendeploy/apps/my-app/releases/OLD_RELEASE
```

## Troubleshooting Failed Deployments

### Check Logs
```bash
frankendeploy logs production --tail 200
```

### Check Container Status
```bash
frankendeploy app status production
```

### Connect to Container
```bash
frankendeploy shell production
```

### Check Health Endpoint
```bash
frankendeploy exec production curl -v http://localhost/health
```

## Recovery Strategies

### If Container Won't Start

1. Check logs: `frankendeploy logs production`
2. Rollback: `frankendeploy rollback production`
3. Fix the issue locally
4. Deploy again

### If Database Migration Fails

1. FrankenDeploy auto-rolls back
2. Check migration locally: `php bin/console doctrine:migrations:status`
3. Fix migration
4. Deploy again

### If App Starts But Errors

1. Check logs for errors
2. Rollback if needed: `frankendeploy rollback production`
3. Debug locally with same configuration
4. Deploy fix

## Best Practices

1. **Always have a health endpoint** - Quick detection of issues
2. **Keep multiple releases** - At least 5 for safe rollbacks
3. **Test locally first** - Use `frankendeploy dev up`
4. **Use staging** - Deploy to staging before production
5. **Monitor after deploy** - Watch logs for a few minutes
