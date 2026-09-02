---
title: Several Apps on One VPS
description: Host more than one application on the same server, isolated from each other
---

A 2 vCPU VPS runs several small Symfony applications without noticing. FrankenDeploy is built for it: every app gets its own directory, database, domain, Caddy configuration and Docker network, and shares nothing but the machine and the reverse proxy.

## Deploying a second app

Nothing special. From the second project's directory:

```bash
frankendeploy init --domain other-app.com
openssl rand -hex 32 | frankendeploy env set prod APP_SECRET --from-stdin
frankendeploy deploy prod
```

`prod` is the same server entry; the app name comes from `frankendeploy.yaml` (`name:`) and must be unique on the server. Caddy learns the new domain at the first deploy and requests its certificate. The first app is not restarted, not even reloaded: Caddy picks up the new configuration with a hot reload.

## What each app gets

| | Per app | Shared |
|---|---|---|
| Directory | `/opt/frankendeploy/apps/<name>/` | |
| Database | `<name>-db` container, own credentials, own volume | |
| Network | `frankendeploy-<name>`: the app, its worker and its database. Nothing else can reach them | Caddy, attached to every app network |
| Domain | `deploy.domain`, one `.caddy` file, one certificate | Ports 80 and 443, the Caddy container |
| Secrets | `shared/.env.local` | |
| Releases, backups | `keep_releases` each | |

Two apps cannot talk to each other over the network, even with the right credentials: they are not on the same Docker network. If you want two of your apps to communicate, do it over HTTPS through their public domains, like any external client would.

## Seeing what runs

```bash
frankendeploy app list prod
```

```
Applications on prod:

  my-app
    Status:  Up 2 hours (healthy)
    Release: 20260902-131358

  other-app
    Status:  Up 5 minutes (healthy)
    Release: 20260902-140102
```

`frankendeploy server status prod` adds CPU and memory per container, and the machine's own load, memory and disk.

## Commands are per project

`deploy`, `rollback`, `logs`, `exec`, `shell`, `env`, `doctor` act on the application of the `frankendeploy.yaml` in the current directory. Run them from the right project; the server argument is the same.

`app` commands take the app name explicitly and work from anywhere:

```bash
frankendeploy app status prod other-app
frankendeploy app remove prod other-app --force              # containers, network, images, directory, database volume
frankendeploy app remove prod other-app --force --keep-data  # same, but the database volume stays
```

## Resources

Every app container can be capped, per app, in its `frankendeploy.yaml`:

```yaml
deploy:
  memory_limit: 512m
  cpu_limit: "1"
```

Without limits, apps share the machine on a first-come basis, and a runaway app can starve the others. With a few apps on a small VPS, a memory limit per app is the one setting worth adding. Log rotation is always on, so logs cannot fill the disk; `keep_releases` bounds the images and backups of each app.

## Domains and subdomains

Each app has one `deploy.domain`. `api.my-app.com` and `my-app.com` are two apps if they are two Symfony projects, or one app with routing inside Symfony if they are the same project (Caddy sends everything for the domain to the container). A redirect from `www.my-app.com` is not generated; add a `.caddy` file for it in `/opt/frankendeploy/caddy/apps/` by hand (`docker exec caddy caddy reload --config /etc/caddy/Caddyfile` after).

## Staging next to production

Same server, two entries in the global config? No: a server entry is a machine, and an app name must be unique per machine. Two options:

- **Two machines**: `frankendeploy server add staging ...`, then `frankendeploy deploy staging`. The cleanest, and what a small team usually does.
- **One machine, two names**: a second `frankendeploy.yaml` is not needed; use `frankendeploy init --name my-app-staging --domain staging.my-app.com --force` in a copy of the project, or maintain two config files and pass `--config frankendeploy.staging.yaml`. Both apps then live side by side, isolated like any two apps.
