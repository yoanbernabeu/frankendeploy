---
title: Quick Start
description: The ten commands, for people who know Docker and SSH
---

You have a Symfony project, a VPS on Ubuntu or Debian you can `ssh` into as root (or with passwordless `sudo`), and a domain whose A record points to it. Here is everything.

```bash
# 1. In the project: detect PHP, extensions, database, assets, Messenger, worker mode
frankendeploy init --domain my-app.com

# 2. Declare and prepare the server: Docker, UFW, Fail2ban, Caddy with Let's Encrypt
frankendeploy server add prod root@203.0.113.42
frankendeploy server setup prod --email you@example.com

# 3. Check local Docker, SSH, sudo, server, disk and DNS; every failure names its fix
frankendeploy doctor prod

# 4. Production secrets live on the server (DATABASE_URL is generated with a managed database)
openssl rand -hex 32 | frankendeploy env set prod APP_SECRET --from-stdin

# 5. Blue-green deploy: build, transfer, migrate, health check, switch, Caddy
frankendeploy deploy prod            # add --remote-build from Apple Silicon to an x86 VPS

# 6. Day two
frankendeploy logs prod -f
frankendeploy env set prod MAILER_DSN --from-stdin --reload
frankendeploy rollback prod
frankendeploy app status prod
```

## What you get

| | |
|---|---|
| Image | Multi-stage Dockerfile on `dunglas/frankenphp`, non-root, cache warmed at build, `HEALTHCHECK` on your health path, FrankenPHP worker mode when the runtime supports it |
| Server | `/opt/frankendeploy/apps/<app>/{releases,current,shared}`, one Docker network per app, Caddy attached to it, nothing but 80/443 published |
| Deploy | New container under a temporary name, DB dump, `pre_deploy` hooks, health check, rename-based swap, worker restart, Caddy reload, retention of images, releases and backups |
| Failure | Anything failing before the swap leaves the old container serving; `--force` overrides hook and health failures |
| Secrets | `shared/.env.local`, `chmod 600`, mounted read-only; `SYMFONY_TRUSTED_PROXIES` injected so Symfony sees HTTPS and the client IP |

## Where the details are

- [Under the Hood](/frankendeploy/under-the-hood/): the server layout, the exact `docker run`, the generated Caddyfile, the deploy sequence step by step
- [Limits & Non-Goals](/frankendeploy/limits/): what it is not designed for
- [frankendeploy.yaml reference](/frankendeploy/config/project/): every field and its default
- [Global config](/frankendeploy/config/global/): servers, SSH authentication, `remote_build`
- [CI/CD](/frankendeploy/guides/ci-cd/): `FRANKENDEPLOY_SSH_KEY`, `--yes`, `doctor` as a gate
- [Security Model](/frankendeploy/security/): what is done, what is deliberately not
- [Troubleshooting](/frankendeploy/guides/troubleshooting/): every error message, cause and fix

## Local development (optional)

```bash
frankendeploy build     # Dockerfile, compose.yaml, entrypoint, .dockerignore
frankendeploy dev up    # http://localhost:8000, database, Mailpit, RabbitMQ when needed
```

Same image as production, `frankenphp_dev` target. See [Local Development](/frankendeploy/guides/local-development/).
