---
title: Limits & Non-Goals
description: What FrankenDeploy is not designed for, so you can decide before you start
---

FrankenDeploy targets one common situation: a Symfony application, a VPS, a small team or a single person, and the wish to deploy without a platform in between. Outside that situation, here is what to expect.

## By design

| | FrankenDeploy | If you need more |
|---|---|---|
| **Servers per app** | One. No load balancing, no failover, no horizontal scaling | Kubernetes, a PaaS, or a container orchestrator with a real load balancer |
| **Database** | One container on the same VPS, one volume, no replication, no automatic failover. Backups are dumps on the same disk | A managed database at your provider, and `database.managed: false` |
| **Zero downtime** | For the application, through the container swap. Not for the database container itself (a PostgreSQL major upgrade stops it) | Plan database upgrades as maintenance |
| **Framework** | Symfony (detection requires `symfony/framework-bundle`), PHP >= 8.2 as FrankenPHP requires | Other PHP frameworks may work with a hand-written `frankendeploy.yaml` and Dockerfile, untested |
| **Server OS** | Ubuntu or Debian, `apt` based, x86_64 or arm64 | Anything else is refused by `server setup` on purpose |
| **Client OS** | macOS, Linux, Windows (the CLI is a single binary) | |
| **Ingress** | HTTP and HTTPS through Caddy. No raw TCP or UDP exposure of your containers (no public database port, no custom TCP service) | Publish it yourself with `docker run -p` outside FrankenDeploy |
| **Domains** | One `deploy.domain` per app, served with HTTPS. No automatic `www` redirect, no wildcard | Add a `.caddy` file by hand in `/opt/frankendeploy/caddy/apps/` |
| **Apps per server** | As many as the machine handles. Docker's default address pools cap bridge networks at about 30 | `default-address-pools` in `/etc/docker/daemon.json`, see the error message |
| **Secrets** | A `.env.local` file per app on the server, `chmod 600` | Symfony's secrets vault works on top of it; no Vault or cloud secret manager integration |
| **Scheduled tasks** | Through Symfony Scheduler and the Messenger worker | A crontab on the host is yours to manage |
| **Assets** | npm (`npm ci`), AssetMapper. Yarn and pnpm are detected but the Node stage currently runs `npm ci` and needs a `package-lock.json` | `npm install --package-lock-only` for now |
| **SQLite** | Supported as a file in a shared directory, not as a managed database | |
| **Custom Dockerfile** | You can edit the generated one; it is never overwritten. FrankenDeploy assumes its contract (port 8080, `app` user, `docker-entrypoint`) | A validation of custom Dockerfiles is [in progress](https://github.com/yoanbernabeu/frankendeploy/issues/91) |
| **Windows Server, Docker Swarm, Podman** | Not supported | |

## Numbers

- A deploy takes about a minute after the first one on a 2 vCPU VPS (image build with a warm cache, transfer, health check). The first one downloads the base images: several minutes.
- A Symfony app in worker mode on a 2 vCPU VPS served 53 requests per second at p95 0.83 s in a load test (23 req/s without worker mode). Your numbers depend on your app.
- 5 releases, 5 images and 5 database dumps are kept by default; each image is 500 MB to 1 GB. `keep_releases` drives all three.

## Stability

FrankenDeploy is in its 0.x versions. The commands and `frankendeploy.yaml` are stable in practice, but a minor version can change what happens on the server (0.15 moved apps to their own networks, 0.16 changed the environment of the container). Every change of that kind is documented in [Upgrading](/frankendeploy/upgrading/) and in the [changelog](https://github.com/yoanbernabeu/frankendeploy/blob/main/CHANGELOG.md), and is applied transparently at the next deploy.
