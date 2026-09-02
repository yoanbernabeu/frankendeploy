---
title: Preflight Checks
description: Diagnose your machine, your server and your DNS before deploying
---

## Why

Most first deployments fail for boring reasons: Docker missing on the server, a `sudo` that asks for a password, a domain whose A record still points to the old host. `frankendeploy doctor` finds them **before** the deploy, and every failed check names the command that fixes it.

```bash
frankendeploy doctor prod
```

## What it checks

**Locally**
- Docker CLI installed and daemon reachable (needed for local builds and the dev environment)

**On the server, over SSH**
- Passwordless `sudo` (unless you connect as root)
- Docker installed and usable without `sudo`
- The `frankendeploy` network created by `server setup`
- The Caddy reverse proxy container running
- Free disk space (a build or an image transfer needs a few GB)

**For the application, when run inside a project**
- The app network `frankendeploy-<app>` exists, Caddy is attached to it, and no container of the app is left on the shared network (see [Network isolation](/frankendeploy/guides/deployment/#network-isolation))

**DNS**
- The domain (`deploy.domain`, or `--domain`) resolves to the public IP of the server. The IP is asked to the server itself, so a VPS behind a gateway is handled. This is the number one cause of Let's Encrypt failures.

## Reading the report

```
✅ Local Docker — daemon 29.4.0
✅ SSH connection — deploy@203.0.113.42:22
✅ Remote sudo — connected as root
❌ Remote Docker — docker: command not found
      Run 'frankendeploy server setup <name> --email you@example.com' to install Docker and Caddy.
❌ Docker network — network "frankendeploy" missing
      Run 'frankendeploy server setup <name> --email you@example.com', or create it manually:
        docker network create frankendeploy
❌ Caddy proxy — caddy container not running
      Run 'frankendeploy server setup <name> --email you@example.com' to (re)start the reverse proxy.
✅ Disk space — 41.5 GB free (9% used)
❌ DNS my-app.com — resolves to 198.51.100.7, server is 203.0.113.42
      Point the A record of my-app.com to 203.0.113.42, then wait for the DNS to propagate.

Error: doctor found blocking problems
```

- ❌ is blocking: `doctor` exits with code 1, which makes it usable as a CI gate.
- ⚠️ is a warning: the deploy can proceed, but something deserves a look.
- With everything green, the report ends with `Everything looks good — ready to deploy!`.

## When to run it

- After `server setup`, to confirm the server is ready
- After changing a DNS record, before the first deploy of a domain
- Whenever a deploy fails: the error messages of `deploy` point to `doctor`

## Options

| Option | Description |
|--------|-------------|
| `--domain` | Domain to check instead of `deploy.domain` from `frankendeploy.yaml` |

`doctor` never changes anything on the server.
