---
title: Troubleshooting
description: Every error message, what it means, and the command that fixes it
---

FrankenDeploy tries to say what is wrong and what to do about it. This page collects the messages you can meet, in the order you would meet them, with a little more context than fits in a terminal. Not finding yours? `frankendeploy doctor prod` is the first thing to run: it checks the machine, the server and the DNS and explains every failure.

## Connecting to the server

### `failed to connect to 203.0.113.42: ...`

The SSH connection itself failed. In order of likelihood:

- **Wrong user, host or port**: `frankendeploy server list` shows what is saved. Fix with `frankendeploy server remove prod` then `server add` again.
- **The key is not on the server**: `ssh -v root@203.0.113.42` from your terminal tells you which keys were tried. Register yours with `ssh-copy-id root@203.0.113.42`.
- **A firewall in front of the server** blocks port 22 (some providers ship one): open it in the provider's console.

### `no usable SSH credentials for prod` / `no working SSH key found`

FrankenDeploy found no key that the server accepts. It tries ssh-agent, then `key_path` from the config, then `~/.ssh/id_ed25519`, `~/.ssh/id_rsa` and the other keys in `~/.ssh/`. Point it at the right one:

```bash
frankendeploy server add prod root@203.0.113.42 --key ~/.ssh/my_key
```

### `key ~/.ssh/id_ed25519 is passphrase-protected and no terminal is available to prompt`

You run with `--yes` or in CI, and the key is encrypted, so nobody can type the passphrase. Either load the key in ssh-agent (`ssh-add ~/.ssh/id_ed25519`) before running, or in CI use an unencrypted deploy key through `FRANKENDEPLOY_SSH_KEY`.

### `The authenticity of host '203.0.113.42' can't be established.`

Normal on the first connection: SSH shows the server's fingerprint and asks you to confirm. Answer `yes`; the key is recorded in `~/.ssh/known_hosts`. In CI, where nobody can answer, provide `FRANKENDEPLOY_KNOWN_HOSTS` with the content of a `known_hosts` file (get it with `ssh-keyscan 203.0.113.42`).

### `host key for 203.0.113.42 has changed (fingerprint: SHA256:...)`

The server presents a different key than the one recorded. If you reinstalled or recreated the VPS, that is expected: `ssh-keygen -R 203.0.113.42`, then run the command again and confirm the new fingerprint. If you did not, stop and investigate: someone may be intercepting the connection.

## Preparing the server

### `unsupported Linux distribution: ...`

`server setup` only knows Ubuntu and Debian (it uses `apt` and their package names). Reinstall the VPS with Ubuntu 24.04 or Debian 12; every provider offers them.

### `passwordless sudo is required for server setup.`

You connect as a user who must type a password for `sudo`. Either connect as `root`, or give the user passwordless sudo on the server:

```bash
echo "deploy ALL=(ALL) NOPASSWD:ALL" | sudo tee /etc/sudoers.d/deploy
```

### `--email is required (used for Let's Encrypt certificates)`

```bash
frankendeploy server setup prod --email you@example.com
```

Let's Encrypt uses it to warn you about certificate problems. Any real address works.

### I ran `server setup` and lost SSH access

That should not happen: the firewall step allows your SSH port (the one you connect to, and the one `sshd` listens on) before enabling UFW. If it did, use your provider's console (a web-based terminal that bypasses SSH) and run `ufw allow 22/tcp`. Then please [open an issue](https://github.com/yoanbernabeu/frankendeploy/issues) with your setup: it is a bug.

## Checking with doctor

### `❌ Remote Docker — docker not found on the server`

The server was not prepared: `frankendeploy server setup prod --email you@example.com`.

### `❌ Docker network — network "frankendeploy" missing` / `❌ Caddy proxy — caddy container not running`

Same cause, same fix. `server setup` is safe to run again on a prepared server; it recreates what is missing.

### `❌ DNS my-app.com — domain does not resolve` / `resolves to 198.51.100.7, server is 203.0.113.42`

The A record of the domain does not point to the server yet. Create or fix it at your DNS provider, then wait: propagation takes from minutes to hours. `dig +short my-app.com` shows what the world currently sees. Your own machine may cache an old answer longer than the rest of the Internet; `doctor` resolves from your machine, so a stale cache can make it say no while the record is right.

### `❌ Local Docker — docker CLI not found`

Docker is not installed on your machine. You need it for local builds and the dev environment. If you deploy with `--remote-build` (or `remote_build: true` on the server), the image is built on the server and local Docker is not used; `doctor` still reports the missing CLI.

## Deploying

### `Missing required environment variables: APP_SECRET (Symfony security secret)`

Symfony refuses to start without `APP_SECRET`, so FrankenDeploy refuses to deploy without it. Run the command printed under the message:

```bash
openssl rand -hex 32 | frankendeploy env set prod APP_SECRET --from-stdin
```

`DATABASE_URL` is required too when `database.managed` is `false` (external database).

### `Architecture mismatch: local arm64 → server x86_64`

You build on an Apple Silicon Mac for an Intel/AMD server; the image would not run there. In interactive mode FrankenDeploy offers to build on the server and remembers the choice. In CI (`--yes`), say it explicitly once:

```bash
frankendeploy server set prod remote_build true
```

### `deployment failed health check: health check failed on my-app-new: HTTP check failed (status: 500) (after 30 attempts)`

The new container started but did not answer 200 on the health path (`/` by default, `/api` for API Platform). FrankenDeploy prints the container's last 50 log lines right above this message: the cause is there (a missing variable, a failed database connection, a PHP fatal). The previous version is still serving; fix and deploy again.

- **404**: the health path does not exist in your app. Set `deploy.healthcheck_path` to a route that does (an API-only app answers 404 on `/`).
- **500**: read the logs. `frankendeploy logs prod` shows the live container; the failed one's logs were printed by the deploy.
- **The app is just slow to start**: widen the window with `deploy.healthcheck_timeout` (90 seconds by default).

### `deployment failed health check: health check failed on my-app-new: container not running (status: exited) (after 1 attempts)`

The container died at startup. The printed logs say why; the usual suspects are a wrong `DATABASE_URL` for an external database, or a `pre_deploy` hook that needs a variable you have not set.

### `pre-deploy hooks failed: ...`

A `pre_deploy` command (typically the migrations) exited with an error, inside the new container, before any traffic switch. The output of the command is shown. The old version still serves. With a managed database, the dump taken just before is listed above, in case the migration went half-way (MySQL and MariaDB do not run DDL in a transaction).

### `database backup failed (use --force to deploy without a backup)`

The dump before the migration did not succeed, so the deploy stopped rather than migrate without a safety net. `frankendeploy logs prod --service app` will not help here: check the database container with `ssh` and `docker logs my-app-db`. Disk full is the classic cause (`doctor` reports free space).

### `database volume my-app-db-data exists but .../.db_credentials is missing`

The database data is there but the file holding its credentials is gone, so FrankenDeploy cannot start a container that matches the data. Either restore `shared/.db_credentials` from a backup of the server, or, if the data does not matter, remove both and let the next deploy start fresh (the message gives the exact commands).

### `reverse proxy configuration failed — the application is running on the server but NOT publicly reachable`

The deploy succeeded but Caddy could not load the configuration for your domain, on the app's first public exposure. The Caddy error is shown. Most often the domain is not in a form Caddy accepts; check `deploy.domain`. `docker logs caddy` on the server has the details.

### `swap failed: ...`

The rename that hands the traffic to the new container failed. FrankenDeploy restores the old container under its name, so the site keeps working, and removes the new one. This is rare (a Docker daemon problem); run the deploy again, and `doctor` if it persists.

### The deploy succeeded, the site shows the previous version

Your browser, or a CDN in front of the server, is caching. `curl -sI https://my-app.com` from a terminal shows what the server answers. FrankenDeploy switches traffic when the health check passes; `frankendeploy app status prod` shows which release is live.

### The deploy succeeded, the site shows "Your connection is not private"

The certificate has not been issued yet. Caddy requests it from Let's Encrypt when it sees the domain, which takes a few seconds; if it fails, the reason is in `docker logs caddy` on the server. Almost always: the domain does not point to this server (`doctor` checks it), or port 80 is not reachable from the Internet (a provider firewall).

## Rolling back

### `no previous release available`

Only one release is on the server, there is nothing to go back to. Releases accumulate with deploys (five are kept by default).

### `release '20260901-140326' not found. Available releases: ...`

The release was removed by retention, or the tag is misspelled. The list shows what is available.

### `rollback aborted, current version untouched: ...`

The release you want to go back to failed its health check when started. Rather than switch to a broken version, FrankenDeploy kept the current one. The printed logs say why the old release fails today (a migration it does not know, a variable it needs).

## Environment variables

### I changed a variable and nothing happened

Without `--reload`, a change applies at the next deploy. `frankendeploy env set prod KEY=value --reload` restarts the app right away, without downtime.

### `env set` refuses my value

Keys must be valid environment variable names (`[A-Z_][A-Z0-9_]*`). Values with spaces or special characters are fine when quoted, or passed with `--from-stdin`.

## Still stuck

- `frankendeploy deploy prod --verbose` prints every command run on the server, with secrets masked.
- `frankendeploy shell prod` opens a shell inside the running container.
- `ssh root@203.0.113.42` then `docker ps` and `docker logs <container>` show the raw state.
- [Open an issue](https://github.com/yoanbernabeu/frankendeploy/issues) with the `--verbose` output: the messages are designed to be pasted.
