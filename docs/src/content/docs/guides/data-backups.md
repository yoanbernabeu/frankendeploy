---
title: Data & Backups
description: What persists on the server, what is backed up for you, and how to restore
---

A deploy replaces the application. Everything that must survive it lives in a few known places. This page lists them, what FrankenDeploy backs up on its own, and the commands to back up and restore the rest.

## What persists, where

| Data | Where on the server | Survives a deploy | Survives `app remove` |
|---|---|---|---|
| Database (managed) | Docker volume `<app>-db-data` | yes | only with `--keep-data` |
| Database dumps | `/opt/frankendeploy/apps/<app>/shared/backups/` | yes | no |
| Secrets | `shared/.env.local`, `shared/.db_credentials` | yes | no |
| Logs, sessions | `shared/var/log/`, `shared/var/sessions/` | yes | no |
| User uploads | `shared/<dir>` for each `deploy.shared_dirs` entry | yes | no |
| Everything else in the container | the image | **no**, replaced by the new image | |

The last line is the rule to remember: a file written by the app outside a shared directory disappears at the next deploy.

## User uploads

Add the directory to `frankendeploy.yaml` before the first upload:

```yaml
deploy:
  shared_dirs:
    - var/log
    - var/sessions
    - public/uploads
```

(setting the list replaces the default, hence the first two lines). The directory is created under `shared/` and mounted into every container at the same path; the app writes there as usual. Files uploaded before the directory was shared are inside an old container and are lost at the next deploy: copy them out first (`docker cp my-app:/app/public/uploads .` on the server).

## Database dumps

With a managed database, FrankenDeploy dumps it before every `pre_deploy` hook that runs migrations:

```
/opt/frankendeploy/apps/my-app/shared/backups/pgsql-20260902-131358.sql.gz
```

- PostgreSQL: `pg_dump --clean --if-exists`, MySQL/MariaDB: `mysqldump --single-transaction --routines --triggers`; gzipped, `chmod 600`, verified non-empty
- `keep_releases` dumps are kept (5 by default), older ones removed after each deploy
- A failed dump aborts the deploy (`--force` to override)

This is a safety net for migrations, not a backup strategy: the dumps are on the same disk as the database, and only taken when a migration runs.

### Taking a dump yourself

On the server, the same commands FrankenDeploy uses. The credentials are in `shared/.db_credentials` (a `DATABASE_URL`):

```bash
# PostgreSQL
docker exec my-app-db pg_dump -U my-app --clean --if-exists my_app | gzip > my_app-$(date +%F).sql.gz

# MySQL / MariaDB
docker exec my-app-db mysqldump -umy-app -p'<password>' --single-transaction --routines --triggers my_app | gzip > my_app-$(date +%F).sql.gz
```

The database name is the app name with hyphens replaced by underscores; the user is the app name.

### Restoring a dump

```bash
# PostgreSQL
gunzip -c my_app-2026-09-02.sql.gz | docker exec -i my-app-db psql -U my-app my_app

# MySQL / MariaDB
gunzip -c my_app-2026-09-02.sql.gz | docker exec -i my-app-db mysql -umy-app -p'<password>' my_app
```

The `--clean --if-exists` options of the PostgreSQL dump drop and recreate every object, so restoring on top of a newer schema works. Stop the worker first if it processes messages (`docker stop my-app-worker`; the next deploy recreates it).

## Off-site copies

Copy the shared directory somewhere else on a schedule; it contains the dumps, the secrets and the uploads:

```bash
# from your machine or a backup host, every night
rsync -az --delete root@203.0.113.42:/opt/frankendeploy/apps/my-app/shared/ ./backups/my-app/shared/
```

To include a fresh dump rather than the last pre-migration one, run the dump command above before the `rsync` (a one-line cron on the server), or dump from the backup host over SSH:

```bash
ssh root@203.0.113.42 "docker exec my-app-db pg_dump -U my-app --clean --if-exists my_app | gzip" > my_app-$(date +%F).sql.gz
```

Your provider's VPS snapshots are a good complement (the whole machine, including the Docker volumes), not a replacement: they are usually daily, and restoring one means restoring everything.

## Rebuilding a server from a backup

The order matters because of the database credentials.

1. New VPS, `frankendeploy server add prod root@<new-ip>` (`ssh-keygen -R <old-ip>` if the IP is reused), `frankendeploy server setup prod --email …`
2. Restore the shared directory **without** `.db_credentials`: `.env.local`, uploads, whatever you keep. Update the DNS.
3. `frankendeploy deploy prod`: the database is created empty, with fresh credentials, and the app starts
4. Restore the dump into it with the commands above
5. Redeploy or restart the worker if you stopped it

Why not restore `.db_credentials`? Because the volume it describes does not exist on the new machine: FrankenDeploy would refuse to start a database that does not match. Fresh credentials plus a dump is the clean path.

## External database

With `database.managed: false`, FrankenDeploy neither creates nor dumps the database: `pre_deploy` migrations run against it without a safety net. Take your own dump before deploying a migration, with your provider's tooling.

## SQLite

The database is a file (`database.path`, `var/data.db` by default) in a directory that `init` adds to `shared_dirs`. It survives deploys; back it up by copying the file from `shared/` while the app is idle, or with `sqlite3 var/data.db ".backup /tmp/copy.db"` inside the container for a consistent copy.

## Removing an app

```bash
frankendeploy app remove prod my-app --force             # everything, database volume included
frankendeploy app remove prod my-app --force --keep-data # keeps my-app-db-data
```

Without `--force`, the command only explains what it would do. The dumps in `shared/backups/` are removed with the directory in both cases: copy them first if you need them.
