---
title: Rollback & Recovery
description: How to roll back to a previous release
---

## Quick Rollback

```bash
frankendeploy rollback prod
```

This switches traffic back to the previous release with the **same guarantees as a deployment**:
- The rollback container gets the same shared directories, `.env.local` and managed `DATABASE_URL` as a regular deploy, and runs on the same isolated network
- A health check runs **before** the swap: if the old release is unhealthy, the rollback aborts and the current version keeps running
- The swap itself is zero-downtime (same rename-based mechanism as deploy)
- The Messenger worker is rolled back to the same release

A rollback only works while the release's Docker image is still on the server, which is what `keep_releases` guarantees.

## Rollback to a Specific Release

List the releases kept on the server:

```bash
frankendeploy app status prod
```

```
Application: my-app
Server:      prod

Status:      Up 2 hours (healthy)
Release:     20260902-143052
Started:     2026-09-02T14:31:10Z

Recent releases:
  * 20260902-143052
    20260902-120000
    20260901-180000
    20260901-150000
    20260831-100000
```

Then:

```bash
frankendeploy rollback prod 20260901-180000
```

## What a Rollback Does Not Undo

**The database schema.** Migrations run during a deploy stay applied. With a managed database, a dump is taken before every migration (`/opt/frankendeploy/apps/<app>/shared/backups/`), and the deploy output tells you where it is when a rollback may not be enough:

```bash
gunzip -c backups/pgsql-<tag>.sql.gz | docker exec -i <app>-db psql -U <user> <db>
```

Write backward-compatible migrations (expand/contract) and a code rollback stays safe. See [Automatic Database Backup](/frankendeploy/guides/deployment/#automatic-database-backup-before-migrations).

**Environment variables.** `.env.local` is shared between releases: a variable changed with `env set` stays changed.

## Automatic Protection During a Deploy

During a deployment, the running version is protected when:

1. The database backup fails
2. A `pre_deploy` hook (migration) fails
3. The health check fails
4. The container swap fails

In every case the new container is removed and the **old container keeps serving traffic untouched**: nothing to restore, because traffic never left the working version.

```
⚠️  Health check failed, rolling back...
```

The last 50 log lines of the failed container are printed so you see the cause immediately.

## Managing Releases

```yaml
deploy:
  keep_releases: 10
```

Releases, Docker images and database backups beyond this count are removed after each deploy. There is nothing to clean manually.

## Troubleshooting a Failed Deployment

```bash
frankendeploy logs prod --tail 200              # app logs
frankendeploy logs prod --service worker        # worker logs
frankendeploy app status prod                   # container and release state
frankendeploy shell prod                        # a shell in the running container
frankendeploy exec prod curl -s http://localhost:8080/health   # the health endpoint, from inside
frankendeploy doctor prod                       # server, network and DNS
```

The app listens on port **8080** inside the container; Caddy handles 80 and 443 in front of it.

## Recovery Strategies

### The New Version Fails Its Health Check
Nothing to do on the server: the old version is still live. Read the log lines printed by the deploy, fix, deploy again.

### The Deploy Succeeded but the App Misbehaves
```bash
frankendeploy logs prod -f
frankendeploy rollback prod
```
Then fix locally and deploy again.

### A Migration Failed Halfway
PostgreSQL runs DDL in a transaction: a failed migration leaves the schema untouched. MySQL and MariaDB do not: check `doctrine:migrations:status` from `frankendeploy shell prod`, and use the pre-migration backup if the schema is partial.

## Best Practices

1. **Have a real health endpoint**: one that touches the database, so a broken deploy is caught before the switch
2. **Keep several releases**: 5 is a good default
3. **Write backward-compatible migrations**: the rollback then stays safe
4. **Deploy to a staging server first** (`frankendeploy deploy staging`): same commands, another server entry
5. **Watch the logs** for a few minutes after a deploy
