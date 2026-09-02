---
title: Upgrading
description: How to update the CLI, and what each version changes on your server
---

## Updating the CLI

```bash
brew upgrade frankendeploy                       # Homebrew
curl -fsSL https://raw.githubusercontent.com/yoanbernabeu/frankendeploy/main/scripts/install.sh | sh   # install script
go install github.com/yoanbernabeu/frankendeploy/cmd/frankendeploy@latest                                # Go
frankendeploy --version
```

Nothing is installed on the server for FrankenDeploy itself: the CLI runs everything over SSH with standard Docker commands. Updating the CLI is enough; the next `deploy` applies whatever changed.

## What a new version changes on the server

FrankenDeploy never changes a running application by itself. Changes of behaviour land on your server at your next `deploy` (or `rollback`, or `env --reload`), and are designed to be transparent. Here is what to expect, most recent first.

### 0.16

**Symfony trusts Caddy.** The app container now receives `SYMFONY_TRUSTED_PROXIES` and `TRUSTED_PROXIES` set to the private subnets. Symfony 7.2+ picks it up without configuration: absolute URLs switch to `https://`, session cookies get the `Secure` flag, the client IP becomes the visitor's. If you already defined either variable with `env set`, nothing changes. If your app is older than Symfony 7.2 and had no `framework.trusted_proxies`, add the wiring shown in [Behind the Proxy](/frankendeploy/guides/deployment/#behind-the-proxy) to benefit.

### 0.15

**One Docker network per application.** The next deploy of an existing app migrates it: the network `frankendeploy-<app>` is created, Caddy attached to it, the database container joins it, the new app container starts on it, and once the old container is stopped the database leaves the shared `frankendeploy` network. No downtime, no data touched. A deploy interrupted in the middle completes on the next one. `frankendeploy doctor` shows the status (`App network`).

**`doctor`** got the `App network` check (0.15.1: only when Docker is present on the server).

### 0.14

**FrankenPHP worker mode with the native Symfony runtime.** Apps on `symfony/runtime` >= 7.4 no longer need `runtime/frankenphp-symfony`; `init` enables worker mode when it detects either. Existing configurations are unchanged.

**No active health check in Caddy (0.14.1).** The generated `.caddy` file no longer probes the live container; per-request timeouts replace it. Applied when the app's Caddy configuration is rewritten, that is at the next deploy.

### 0.13

**`doctor`**, pure-Go SFTP transfers (no `scp`/`rsync` needed), OS detection in `server setup`, and the deployment pipeline extracted and tested per failure scenario. **The default `cache:warmup` post-deploy hook was removed** from what `init` generates (the cache is warmed at image build); an existing hook in your `frankendeploy.yaml` still runs, and can be deleted.

### 0.12

**Automatic database backup before migrations**, image pruning with `keep_releases`, log rotation on every container, optional `memory_limit` / `cpu_limit`.

### 0.11

**SSH**: ssh-agent, passphrase-protected keys, host key verification (`known_hosts`). A server whose host key was never recorded asks for confirmation on the next connection. **Env writes** enforce `chmod 600`; `env set --from-stdin`.

### Before 0.11

See the [changelog](https://github.com/yoanbernabeu/frankendeploy/blob/main/CHANGELOG.md).

## Updating the server

FrankenDeploy does not update the operating system, Docker, or the Caddy image. See the [Security Model](/frankendeploy/security/) for what is yours to schedule. Re-running `frankendeploy server setup prod --email …` is safe at any time and repairs what is missing; it does not upgrade Docker.

To move the Caddy container to a newer `caddy:alpine`, on the server:

```bash
docker pull caddy:alpine
```

then `frankendeploy server setup prod --email …`, which recreates the container (certificates live in the `caddy_data` volume and are kept). The apps are not affected, but Caddy is down for a few seconds.

## Downgrading

Install the previous binary (releases keep every version) and deploy. Server-side changes are additive (a network, environment variables) and harmless to an older CLI.
