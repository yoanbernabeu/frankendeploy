---
title: Introduction
description: What FrankenDeploy is, who it is for, and where to start
---

## In one sentence

**FrankenDeploy** puts a Symfony application online on a server you own, with one command, and keeps it online while you ship new versions.

```bash
frankendeploy deploy prod
```

Behind that command: a Docker image built for your app, a server prepared with a firewall and HTTPS, a health check before every traffic switch, a database backed up before every migration, and a rollback when something goes wrong. You keep a plain VPS, a plain Symfony project, and no vendor to depend on.

## Where to start

<div class="grid grid-cols-1 md:grid-cols-2 gap-4 not-prose my-6">
  <a href="/frankendeploy/before-you-start/" class="block rounded-xl border border-white/10 bg-white/5 p-5 hover:bg-white/10 transition-colors">
    <div class="text-lg font-semibold text-white mb-1">I have never deployed anything</div>
    <div class="text-[#C3B2D3] text-sm">You have a Symfony app that works on your machine and you want it on the Internet. Start with <strong>Before You Start</strong>, then follow <strong>Your First Deployment</strong> step by step. About an hour, no prior server knowledge.</div>
  </a>
  <a href="/frankendeploy/quickstart/" class="block rounded-xl border border-white/10 bg-white/5 p-5 hover:bg-white/10 transition-colors">
    <div class="text-lg font-semibold text-white mb-1">I know Docker and SSH</div>
    <div class="text-[#C3B2D3] text-sm">You want the commands, then the details. The <strong>Quick Start</strong> is ten lines; <a href="/frankendeploy/under-the-hood/" class="underline">Under the Hood</a> and the <a href="/frankendeploy/config/project/" class="underline">frankendeploy.yaml reference</a> tell you exactly what runs on the server.</div>
  </a>
</div>

## What it does

- **Reads your project** and writes one configuration file, `frankendeploy.yaml`: PHP version and extensions, database, assets, Messenger, worker mode, all detected from your code.
- **Prepares a bare Ubuntu or Debian server**: Docker, UFW firewall, Fail2ban, and Caddy as a reverse proxy with automatic Let's Encrypt certificates. `frankendeploy doctor` checks the machine, the server and your DNS, and names the command that fixes each problem.
- **Deploys without downtime**: the new version starts next to the old one, runs its migrations, passes a health check, and only then receives the traffic. If anything fails first, the old version keeps serving and nothing changed for your visitors.
- **Runs the database for you**: PostgreSQL, MySQL or MariaDB in a container, credentials generated and injected, a dump before every migration.
- **Operates the app**: `logs`, `exec`, `shell`, `env set` for secrets applied without downtime, `rollback` to any kept release with the same health check as a deploy.
- **Hosts several apps on one server**, each on its own isolated Docker network.

## What it is not

- **Not a PaaS**: there is no dashboard, no account, no monthly fee. The server is yours, and so is its maintenance (system updates, backups off the machine). The [Security Model](/frankendeploy/security/) says exactly what FrankenDeploy handles and what it leaves to you.
- **Not a cluster**: one VPS, one copy of your app. Enough for most projects; not built for horizontal scaling.
- **Not magic**: it runs standard Docker, Caddy and shell commands on an ordinary Linux server. Everything it does can be read in [Under the Hood](/frankendeploy/under-the-hood/) and in the [source code](https://github.com/yoanbernabeu/frankendeploy).

## Built on FrankenPHP

FrankenDeploy is a deployment layer on top of [FrankenPHP](https://frankenphp.dev), the PHP application server created by Kévin Dunglas: Caddy and PHP in a single binary, HTTP/2 and HTTP/3, and a worker mode that keeps your Symfony kernel in memory between requests. FrankenDeploy generates the Docker image, FrankenPHP runs it.

## Status

FrankenDeploy is open source (MIT) and young: the command set is stable, but breaking changes can still happen between minor versions. Each one is listed in the [changelog](https://github.com/yoanbernabeu/frankendeploy/blob/main/CHANGELOG.md).
