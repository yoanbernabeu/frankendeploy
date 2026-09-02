---
title: FAQ
description: Short answers to the questions people ask after the talk
---

## Do I need Docker on my machine?

Only for two things: building the image locally, and the local development environment (`frankendeploy dev up`). With `--remote-build` (or `remote_build: true` on the server), the image is built on the VPS and your machine needs nothing but the CLI. From an Apple Silicon Mac to an x86 server, remote build is the recommended path anyway.

## Which servers work?

Any VPS running **Ubuntu or Debian**, reachable over SSH as root or as a user with passwordless `sudo`. 1 GB of RAM runs a small site; 2 GB is comfortable, more if you build on the server. The provider does not matter.

## How much does it cost?

FrankenDeploy is free and open source (MIT). A VPS able to run a Symfony app with its database costs a few euros a month. The domain is the other cost.

## Can I keep my existing database?

Yes. Set `database.managed: false` in `frankendeploy.yaml` and give the app its `DATABASE_URL` with `frankendeploy env set prod DATABASE_URL --from-stdin`. FrankenDeploy then leaves the database alone: no container, no automatic backup (do your own before migrating).

## Where are my files on the server?

Under `/opt/frankendeploy/apps/<app>/`: `releases/<tag>/` for each deployed version, `current` pointing to the live one, `shared/` for what persists between versions (`.env.local`, `var/log`, `var/sessions`, your uploads directory if you added it to `shared_dirs`, `backups/` for the database dumps). The database data itself is in a Docker volume, `<app>-db-data`.

## Where do user uploads go?

Add the directory to `deploy.shared_dirs` (for instance `public/uploads`): it is stored under `shared/` on the server and mounted into every release, so a deploy never loses it. Everything else in the container is replaced at each deploy.

## What about cron jobs?

Use [Symfony Scheduler](https://symfony.com/doc/current/scheduler.html): your recurring tasks become messages, consumed by the Messenger worker that FrankenDeploy runs for you (`messenger.enabled: true`, `scheduler_default` added to the transports by `init`). No crontab to maintain on the server, and the worker restarts with every deploy.

## Can I run several apps on one VPS?

Yes, that is a normal setup. Each app has its own directory, database, domain, Caddy configuration and Docker network; they cannot see each other. Deploy each one from its project directory to the same server. `frankendeploy app list prod` shows what runs there.

## Can I deploy to several servers (staging, production)?

Yes: `frankendeploy server add staging ...`, then `frankendeploy deploy staging`. Servers are declared once on your machine and shared by all your projects.

## Does a deploy cause downtime?

No. The new version starts next to the old one, runs its migrations, passes a health check, and only then receives the traffic; the old one is stopped after. If anything fails before the switch, nothing changes for your visitors. The only thing a code rollback does not undo is a database migration, which is why the database is dumped before each one.

## How do I set secrets?

On the server, never in the project: `frankendeploy env set prod KEY --from-stdin`. They land in a file only the app can read. `--reload` applies a change without downtime.

## Is HTTPS automatic?

Yes. Caddy obtains a Let's Encrypt certificate for `deploy.domain` at the first deploy and renews it. The only requirement is a DNS A record pointing to the server; `frankendeploy doctor` checks it.

## Does it work from Windows?

The CLI runs on Windows (a binary is published for each release). Remote builds and SFTP transfers work without any Unix tool. The **server** must be Linux.

## Can I see or change what is generated?

Yes: `frankendeploy build` writes the `Dockerfile`, `docker-entrypoint.sh`, `compose.yaml`, `.dockerignore` (and a `Caddyfile` in worker mode) into your project. Edit them; FrankenDeploy never overwrites a file you changed.

## Is it safe to run `server setup` twice?

Yes. Every step checks the current state; a second run changes nothing on a prepared server, and repairs what is missing on a damaged one.

## What does FrankenDeploy not do for me?

System security updates, off-site backups of the database dumps, monitoring. The [Security Model](/frankendeploy/security/) lists it precisely.

## Can I use it in CI?

Yes. `--yes` removes every prompt, `FRANKENDEPLOY_SSH_KEY` and `FRANKENDEPLOY_KNOWN_HOSTS` carry the credentials, and `doctor` exits 1 on a blocking problem. Recipes for GitHub Actions and GitLab CI are in the [deployment guide](/frankendeploy/guides/deployment/#cicd-integration).

## Something is wrong, where do I look?

[Troubleshooting](/frankendeploy/guides/troubleshooting/) lists every message with its fix. `frankendeploy doctor prod` first, `frankendeploy logs prod -f` second.
