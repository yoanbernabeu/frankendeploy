---
title: Under the Hood
description: Exactly what FrankenDeploy puts on your server and runs at each deploy
---

For people who want to see before they trust. Everything below is read from the source, and everything can be verified on the server with `ssh`, `docker ps`, `docker inspect`. Nothing here is required to use FrankenDeploy.

## On the server

### Files

```
/opt/frankendeploy/
├── apps/
│   └── my-app/
│       ├── releases/
│       │   ├── 20260901-140326/release      ← one directory per kept release
│       │   └── 20260902-131358/release        (a marker file: the code lives in the image)
│       ├── current -> releases/20260902-131358
│       ├── shared/                           ← survives every deploy
│       │   ├── .env.local                    600, mounted read-only in the containers
│       │   ├── .db_credentials               600, the generated DATABASE_URL
│       │   ├── backups/pgsql-<tag>.sql.gz    600, one dump per migration, keep_releases kept
│       │   ├── var/log/
│       │   └── var/sessions/                 + every entry of deploy.shared_dirs
│       └── build/                            ← source tree of the last remote build
└── caddy/
    ├── Caddyfile                             ← global options, imports apps/*.caddy
    ├── apps/my-app.caddy                     ← one file per app, written by deploy
    └── logs/my-app.log                       ← JSON access log, 10 MB × 5
```

Everything under `apps/` belongs to the user you deploy with. The application code is not on disk: it is inside the Docker image, tagged with the release.

### Containers

| Container | Image | Started by | Network(s) | Published ports |
|---|---|---|---|---|
| `caddy` | `caddy:alpine` | `server setup` | `frankendeploy` + every `frankendeploy-<app>` | 80, 443, 443/udp |
| `my-app` | `my-app:<tag>` | `deploy` | `frankendeploy-my-app` | none |
| `my-app-worker` | `my-app:<tag>` | `deploy`, when `messenger.enabled` | `frankendeploy-my-app` | none |
| `my-app-db` | `postgres:16-alpine`, `mysql:<v>`, `mariadb:<v>` | first `deploy`, when `database.managed` | `frankendeploy-my-app` | none |

All of them run with `--restart unless-stopped`, so they come back after a reboot, and with `--log-driver json-file --log-opt max-size=10m --log-opt max-file=3`.

### Networks

`server setup` creates the `frankendeploy` bridge network for Caddy. The first deploy of an app creates `frankendeploy-<app>` and attaches Caddy to it with `docker network connect`. Docker's embedded DNS lets Caddy resolve `my-app` to the container, whatever its IP, and nothing outside the app's network can reach it. See [Network Isolation](/frankendeploy/guides/deployment/#network-isolation) for the migration of installations older than 0.15.

### The app container, verbatim

This is the command `deploy`, `rollback` and `env --reload` run (`buildAppRunCommand` in `internal/cmd/deploy.go`), for a PostgreSQL app with the default shared paths:

```bash
docker run -d --name my-app-new \
  --network frankendeploy-my-app \
  --restart unless-stopped \
  --user 1000:1000 \
  --log-driver json-file --log-opt max-size=10m --log-opt max-file=3 \
  [--memory 512m] [--cpus 1.5] \
  -e SERVER_NAME=:8080 -e APP_ENV=prod -e APP_DEBUG=0 \
  -e DATABASE_URL='postgresql://my-app:<generated>@my-app-db:5432/my_app?serverVersion=16&charset=utf8' \
  -e SYMFONY_TRUSTED_PROXIES=127.0.0.1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16 \
  -e TRUSTED_PROXIES=127.0.0.1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16 \
  -v /opt/frankendeploy/apps/my-app/shared/var/log:/app/var/log \
  -v /opt/frankendeploy/apps/my-app/shared/var/sessions:/app/var/sessions \
  -v /opt/frankendeploy/apps/my-app/shared/.env.local:/app/.env.local:ro \
  my-app:20260902-131358
```

- `--user 1000:1000`: the image's `app` user; nothing in the container runs as root.
- `SERVER_NAME=:8080`: FrankenPHP's own Caddy listens on plain HTTP, port 8080, unprivileged. TLS ends at the front Caddy.
- `.env.local` is mounted read-only: the app cannot alter its own secrets.
- The trusted proxies are skipped when you define either variable in `.env.local` yourself.

The worker container uses the same mounts and `DATABASE_URL`, with the command `php bin/console messenger:consume <transports> --time-limit=3600 --memory-limit=256M -vv`.

### The database container

```bash
docker run -d --name my-app-db \
  --network frankendeploy-my-app \
  --restart unless-stopped \
  --log-driver json-file --log-opt max-size=10m --log-opt max-file=3 \
  -e POSTGRES_USER=my-app -e POSTGRES_PASSWORD=<generated> -e POSTGRES_DB=my_app \
  -v my-app-db-data:/var/lib/postgresql/data \
  postgres:16-alpine
```

The password is 24 random hex characters, generated once and saved in `shared/.db_credentials`. MySQL and MariaDB additionally get `MYSQL_RANDOM_ROOT_PASSWORD=1` / `MARIADB_RANDOM_ROOT_PASSWORD=1`: the app password is never the root password. Data lives in the named volume `my-app-db-data`; `app remove` deletes it unless `--keep-data`.

### The generated Caddy configuration

`server setup` writes `/opt/frankendeploy/caddy/Caddyfile`:

```
{
    admin localhost:2019
    email you@example.com
}

import /config/apps/*.caddy
```

The admin API only listens inside the container: reloads go through `docker exec caddy caddy reload`. Each deploy writes `apps/my-app.caddy`:

```
my-app.com {
    reverse_proxy my-app:8080 {
        transport http {
            dial_timeout 5s
            response_header_timeout 60s
        }
    }

    encode zstd gzip

    header {
        Strict-Transport-Security "max-age=31536000"
        X-Content-Type-Options nosniff
        X-Frame-Options DENY
        Referrer-Policy strict-origin-when-cross-origin
        -Server
    }

    log {
        output file /config/logs/my-app.log {
            roll_size 10mb
            roll_keep 5
        }
        format json
    }
}
```

No active health check on the upstream, deliberately: with a single upstream there is nothing to fail over to, and a probe timing out under load would turn a slow app into a 503 for everyone. The per-request timeouts bound the damage of a frozen container to one 504. HTTPS, HTTP/2, HTTP/3 and the certificate come from Caddy's defaults.

## The image

`frankendeploy build` writes a multi-stage `Dockerfile` (you can read and edit it; FrankenDeploy never overwrites a modified file):

| Stage | Base | What happens |
|---|---|---|
| `frankenphp_upstream` | `dunglas/frankenphp:1-php8.4` | The FrankenPHP release, PHP version from `php.version` |
| `node_build` | `node:22-slim` | Only with npm/yarn/pnpm assets: `npm ci`, then `assets.build_command` |
| `frankenphp_base` | upstream | System packages (`dockerfile.extra_packages`), `install-php-extensions` for `php.extensions` and Composer, `php.ini_values`, the `app` user (uid 1000), the entrypoint, `HEALTHCHECK --start-period=60s curl -f http://localhost:8080<healthcheck_path>` |
| `frankenphp_dev` | base | `APP_ENV=dev`, `XDEBUG_MODE=off`, `composer install` with dev dependencies, `frankenphp run --watch` |
| `frankenphp_prod` | base | Production `php.ini` (opcache without timestamp validation, 256 MB, preload when `config/preload.php` exists), `composer install --no-dev`, the built assets copied from `node_build` or `asset-map:compile`, `cache:warmup`, `USER app` |

`docker-entrypoint.sh` does one thing in production: wait for the database host of `DATABASE_URL` to accept TCP connections, then `exec frankenphp run`. Migrations are not its job; they run as a `pre_deploy` hook, before the traffic switch, where a failure can still abort the deploy.

With `frankenphp.worker: true`, a `Caddyfile` for FrankenPHP is generated and copied into the image: it declares `public/index.php` as the worker script, so the Symfony kernel boots once and stays in memory.

## A deploy, step by step

`deploy.RunPipeline` in `internal/deploy/orchestrator.go` runs the sequence below. What happens on failure is decided once, there, and tested per scenario.

| # | Step | On failure |
|---|---|---|
| 1 | Env pre-flight: `APP_SECRET`, `DATABASE_URL` for an external database | Abort before touching the server |
| 2 | Generate missing artifacts, build the image (locally, or on the server from the transferred source), transfer it | Abort |
| 3 | `EnsureAppNetwork`: create `frankendeploy-<app>`, attach Caddy, attach an older database | Abort |
| 4 | Managed database: start it if stopped, create it on the first deploy | Abort |
| 5 | `prepareRelease`: `releases/<tag>`, `shared/` paths, permissions | Abort |
| 6 | Start `<app>-new` from the new image (the old `<app>` keeps serving) | Abort, remove `<app>-new` |
| 7 | Managed database + migration hook: dump to `shared/backups/` | Abort unless `--force` |
| 8 | `pre_deploy` hooks with `docker exec <app>-new …` | Abort unless `--force`; warn that the schema may already be migrated |
| 9 | Health check: `curl -f http://localhost:8080<path>` inside `<app>-new`, up to 30 attempts over 90 s | Print the container's last 50 log lines, abort unless `--force` |
| 10 | Swap: `docker rename <app> <app>-old`, `docker rename <app>-new <app>`, then stop `<app>-old`; update `current` | Restore `<app>-old` under its name, abort |
| 11 | Recreate the worker on the new image; detach the app from the shared network if it was migrating | Warn |
| 12 | `post_deploy` hooks in `<app>` | Warn |
| 13 | Write `caddy/apps/<app>.caddy`, `caddy reload` | Abort on the app's first exposure (it would be running but unreachable), warn otherwise |
| 14 | Retention: releases, images (`docker rmi`, never forced), backups beyond `keep_releases`; dangling layers after a remote build | Best effort |

The swap is two renames. Caddy proxies to the name `my-app`; Docker's DNS follows the rename instantly, so no request meets a missing upstream. The old container is stopped only after the new one owns the name.

`rollback` runs steps 3, 6, 9, 10, 11 with a kept image; `env --reload` runs 3, 6, 9, 10 with the current image.

## What the CLI keeps on your machine

The global configuration (servers, `remote_build`, `key_path`), in the user configuration directory (see [Global Config](/frankendeploy/config/global/)). Nothing else: no state about deployed releases, which is why any machine with the CLI and the SSH key can deploy or roll back.

## Reading the code

The repository is organised the way this page is:

- `internal/cmd/`: one file per command; `deploy.go` builds the `docker run` commands
- `internal/deploy/`: the pipeline (`orchestrator.go`), health checks, database, backups, networks, env files, retention
- `internal/generator/`: the Dockerfile, entrypoint and Compose templates
- `internal/caddy/`: the two Caddy templates above
- `internal/ssh/`: the pure-Go SSH and SFTP client (no `ssh` binary needed on your machine)
- `internal/scanner/`: what `init` detects in your project

`frankendeploy deploy prod --verbose` prints every command sent to the server, with secrets masked.
