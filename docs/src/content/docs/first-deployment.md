---
title: Your First Deployment
description: A Symfony app on the Internet in one hour, step by step, with the real output of every command
---

We deploy an API Platform application with a PostgreSQL database (the demo app of the API Platform Con 2026 talk) from an empty Ubuntu VPS to `https://my-app.com`. Every terminal block below is the real output of the command, captured on that server; only the application name, the domain and the host are replaced by `my-app`, `my-app.com` and `203.0.113.42`.

Before starting, you need what [Before You Start](/frankendeploy/before-you-start/) describes: a VPS you can `ssh` into, a domain pointing to it, and FrankenDeploy installed. Count one hour the first time, including reading.

## Step 1: Describe the project (2 minutes)

In the project directory:

```bash
frankendeploy init --domain my-app.com
```

```
ℹ️  Analyzing project...
✅ Created frankendeploy.yaml

📋 Project Configuration:
   Name:        my-app
   PHP:         8.4
   Extensions:  ctype, iconv, intl, opcache, zip, pdo_pgsql
   Database:    pgsql 16
   Assets:      assetmapper
   Domain:      my-app.com
   Healthcheck: /api (API Platform detected)

Next steps:
  1. Review frankendeploy.yaml and adjust if needed
  2. Run 'frankendeploy build' to generate Docker files
  3. Run 'frankendeploy dev up' to start local development
```

FrankenDeploy read `composer.json` (PHP 8.4, the extensions), `.env` (PostgreSQL 16), `importmap.php` (AssetMapper) and noticed API Platform, so the health check will hit `/api` rather than `/`. Everything landed in one file, `frankendeploy.yaml`, that you commit with the project. Open it: it is short and every line is explained in the [reference](/frankendeploy/config/project/).

<div class="callout callout-note">

**What just happened**

Nothing on any server yet. `init` only wrote a file. When it makes a choice for you (adding an extension, enabling worker mode), it says so on this screen.

</div>

## Step 2: Declare the server (1 minute)

```bash
frankendeploy server add prod root@203.0.113.42
```

```
✅ Added server 'prod' (root@203.0.113.42)
ℹ️  Testing SSH connection...
✅ SSH connection successful
```

`prod` is the name you will use in every other command. The connection details are saved on your machine, outside the project (nothing about your server goes into Git). On the first connection FrankenDeploy shows the server's fingerprint and asks you to confirm it, like `ssh` does.

## Step 3: Prepare the server (3 minutes)

```bash
frankendeploy server setup prod --email you@example.com
```

```
✅ Connected to 203.0.113.42
ℹ️  Setting up server for FrankenDeploy...
ℹ️  [1/5] Installing prerequisites...
ℹ️  [2/5] Installing Fail2ban...
ℹ️  [3/5] Installing Docker...
ℹ️  [4/5] Configuring FrankenDeploy...
ℹ️  [5/5] Configuring firewall and starting Caddy...
✅ Caddy container is running
✅ Server 'prod' is ready for deployments!

Configuration:
  Email:    you@example.com (for Let's Encrypt)
  Caddy:    Docker container with Admin API
  Docker:   Installed with 'frankendeploy' network
  Firewall: Ports 22, 80, 443 open
  Fail2ban: SSH protection enabled (5 retries, 1h ban)

Next step:
  Run 'frankendeploy deploy prod' from your Symfony project
```

The email is what Let's Encrypt uses to warn you about a certificate problem. In three minutes the server got Docker, a firewall that only lets SSH, HTTP and HTTPS through, Fail2ban against SSH brute force, and Caddy, the reverse proxy that will obtain and renew the HTTPS certificate. You can run this command again later, it changes nothing on a prepared server.

## Step 4: Check everything (30 seconds)

```bash
frankendeploy doctor prod
```

```
✅ Local Docker — daemon 29.4.0
✅ SSH connection — root@203.0.113.42:22
✅ Remote sudo — connected as root
✅ Remote Docker — 29.7.2
✅ Docker network — frankendeploy
✅ Caddy proxy — Up 51 seconds
✅ Disk space — 43.4 GB free (4% used)
✅ App network — frankendeploy-my-app (created by the first deploy)
❌ DNS my-app.com — domain does not resolve
      Create an A record for my-app.com pointing to 203.0.113.42 at your DNS provider.
      DNS changes can take up to a few hours to propagate.

Error: doctor found blocking problems
```

This is the real output of the day: the DNS record had just been created and had not propagated yet. `doctor` is the command to run whenever you wonder where you stand: every red line names what to do. A few minutes later:

```
✅ DNS my-app.com — → 203.0.113.42

✅ Everything looks good — ready to deploy!
```

## Step 5: The first deploy, refused (10 seconds)

```bash
frankendeploy deploy prod
```

```
ℹ️  Deploying my-app to prod...
✅ Connected to 203.0.113.42
ℹ️  Running pre-flight checks...
❌ Missing required environment variables:

   APP_SECRET (Symfony security secret)

Run the following commands to configure them:

   frankendeploy env set prod APP_SECRET=$(openssl rand -hex 32)

Then run 'frankendeploy deploy prod' again.

Or use --force to skip this check (not recommended for production)
```

FrankenDeploy refused before touching anything: Symfony needs `APP_SECRET`, and it is not on the server. Production secrets never live in the project; they live on the server, in a file only the application can read. Run the command it gives you (or the variant below, which keeps the secret out of your shell history):

```bash
openssl rand -hex 32 | frankendeploy env set prod APP_SECRET --from-stdin
```

No `DATABASE_URL` to set: `frankendeploy.yaml` says the database is managed, so FrankenDeploy will create it and pass its address to the app.

## Step 6: The first deploy (1 to 3 minutes)

```bash
frankendeploy deploy prod
```

```
ℹ️  Deploying my-app to prod...
✅ Connected to 203.0.113.42
ℹ️  Using remote build (server configured for cross-architecture)
ℹ️  Running pre-flight checks...
✅ Pre-flight checks passed
ℹ️  Transferring source code to server...
ℹ️    121 files transferred
✅ Source code transferred
ℹ️  Building Docker image on server...
✅ Image built: my-app:20260901-140326
ℹ️  Preparing application network...
✅ Created network frankendeploy-my-app
ℹ️  Setting up managed database...
✅ Database ready: pgsql
ℹ️  Preparing release...
ℹ️  Starting new version (blue-green)...
ℹ️  Backing up database before migration...
✅ Database backup: /opt/frankendeploy/apps/my-app/shared/backups/pgsql-20260901-140326.sql.gz
ℹ️  Running pre-deploy hooks...
ℹ️  Running health check...
✅ Health check passed
ℹ️  Swapping containers...
ℹ️  Updating reverse proxy...
✅ Caddy configured for my-app.com
ℹ️  Cleaning up old releases...
✅ Deployment complete in 1m4s!

Application deployed: my-app
  Tag: 20260901-140326
  URL: https://my-app.com
```

Open the URL: the app answers over HTTPS, certificate included. The very first deploy on a server takes longer (here, the first one took 5 minutes: Docker had to download the base images); the next ones take about a minute.

<div class="callout callout-note">

**What just happened, line by line**

- **Using remote build**: this deploy came from an Apple Silicon Mac to an x86 server. FrankenDeploy noticed on the first attempt, offered to build on the server instead, and remembered the answer. From a Linux or Intel machine, the image is built locally and transferred.
- **Source code transferred, image built**: your project became a Docker image, a self-contained package with PHP, your code, your dependencies and your compiled assets.
- **Created network, database ready**: the app gets its own private network on the server, and a PostgreSQL container with generated credentials. Nothing to configure.
- **Starting new version, backing up, pre-deploy hooks**: the new container starts next to whatever was running before, the database is dumped, then the migrations run.
- **Health check passed**: FrankenDeploy asked the new container for `/api` and got a 200. Only now does it continue.
- **Swapping containers**: traffic moves to the new container. On a redeploy, the old one is stopped only after that, so nobody sees an error.
- **Caddy configured**: the reverse proxy now knows `my-app.com` and requests the certificate.

</div>

## Step 7: Ship a change (1 minute)

Edit anything, commit or not, and run the same command:

```bash
frankendeploy deploy prod
```

Same output, one difference at the end: no `Created network` line, the network exists. During the minute it takes, the previous version keeps answering; the switch happens after the health check. That is the whole point: deploying becomes a habit rather than an event.

## Step 8: Undo it (30 seconds)

Something wrong with that change? Go back:

```bash
frankendeploy rollback prod
```

```
ℹ️  Connecting to 203.0.113.42...
ℹ️  Rolling back from 20260902-131358 to 20260902-115928...
ℹ️  Running health check...
✅ Health check passed
ℹ️  Swapping containers...
✅ Rolled back to release 20260902-115928
```

The previous release is still on the server (five are kept by default), it gets the same health check as a deploy, and the switch is the same zero-downtime swap. `frankendeploy app status prod` lists the releases you can go back to.

## Where you are now

- The app is at `https://my-app.com`, with a certificate that renews itself.
- The server has a firewall, Fail2ban, and only exposes ports 22, 80 and 443.
- The database lives in a Docker volume on the server, and is dumped before every migration.
- Secrets are in `/opt/frankendeploy/apps/my-app/shared/.env.local`, readable only by the app.
- `frankendeploy logs prod -f` shows what the app says; `frankendeploy shell prod` opens a shell inside it.

## What to read next

- [Environment Variables](/frankendeploy/guides/environment-variables/): `MAILER_DSN`, API keys, and applying a change without downtime
- [Deployment](/frankendeploy/guides/deployment/): health checks, hooks, backups, what happens when something fails
- [Troubleshooting](/frankendeploy/guides/troubleshooting/): every error message, its cause and its fix
- [Security Model](/frankendeploy/guides/server-setup/#security-model): what is handled for you, and what stays your job (system updates, off-site backups)
