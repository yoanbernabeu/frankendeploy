---
title: Local Development
description: Use FrankenDeploy for local Symfony development
---

## Development Environment

`frankendeploy build` generates a `compose.yaml` that runs your application on FrankenPHP with the services it needs. The image is the same multi-stage `Dockerfile` as production, built with its `frankenphp_dev` target.

```bash
frankendeploy build      # once, generates compose.yaml
frankendeploy dev up
```

This starts:
- Your Symfony application on **http://localhost:8000** (and https://localhost with a local certificate)
- The database (PostgreSQL, MySQL or MariaDB) with `DATABASE_URL` already set
- [Mailpit](https://mailpit.axllent.org/) when `mailer.enabled` (SMTP on `mailpit:1025`, web UI on **http://localhost:8025**)
- RabbitMQ 4 when a Messenger transport uses `amqp://` (management UI on **http://localhost:15672**)

## Development Commands

```bash
frankendeploy dev up --build      # rebuild the image first (Dockerfile or extensions changed)
frankendeploy dev logs            # follow the logs (default)
frankendeploy dev logs --tail 50  # last 50 lines
frankendeploy dev down            # stop the environment
frankendeploy dev restart         # restart it
```

`dev up` runs in the background by default (`--detach`).

## Volume Mounts

Your project directory is mounted at `/app` in the container, so code changes are visible immediately. `vendor/` is a named Docker volume: dependencies are installed inside the container and never clash with a local `vendor/` from another PHP version. Everything else, including `var/` and `node_modules/`, is shared with your machine.

## Running Symfony Commands

The application service is named `app`:

```bash
docker compose exec app php bin/console cache:clear
docker compose exec app composer require some/package
docker compose exec app php bin/phpunit
```

## Database Access

The dev database uses `app` / `app` as user and password and `app` as database name. Its port is bound to `127.0.0.1` only, so nothing on your network can reach it.

```bash
# PostgreSQL
docker compose exec database psql -U app -d app
# from your machine: postgresql://app:app@127.0.0.1:5432/app

# MySQL / MariaDB
docker compose exec database mysql -u app -papp app
# from your machine: mysql://app:app@127.0.0.1:3306/app
```

Inside the container, `DATABASE_URL` points to the `database` service. Override the host, port or name with `database.host`, `database.port`, `database.name` in `frankendeploy.yaml` if you need to.

## Debugging with Xdebug

PHP extensions listed in `frankendeploy.yaml` are installed in the image (dev and prod). The dev stage ships `XDEBUG_MODE=off`, so listing `xdebug` is harmless until you enable it:

```yaml
php:
  extensions:
    - xdebug
env:
  dev:
    XDEBUG_MODE: debug
    XDEBUG_CONFIG: "client_host=host.docker.internal"
```

Then rebuild:

```bash
frankendeploy build
frankendeploy dev up --build
```

Keep in mind that the extension is also present in the production image, unloaded but installed. Remove it from `extensions` before a release if you prefer a lean image.

## Email Testing

With `mailer.enabled: true` (set by `init` when `config/packages/mailer.yaml` exists), Mailpit catches every email sent by the app. Point the mailer at it, either in `.env.local` or in the `env.dev` section of `frankendeploy.yaml`:

```yaml
env:
  dev:
    MAILER_DSN: "smtp://mailpit:1025"
```

Open http://localhost:8025 to read the messages.

## Assets

With Webpack Encore or Vite, run the dev server on your machine, next to the container:

```bash
npm run dev
# or
yarn dev
```

With AssetMapper, changes are served directly by Symfony in dev mode.
