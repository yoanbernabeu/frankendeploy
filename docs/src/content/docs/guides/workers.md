---
title: Workers & Scheduled Tasks
description: Asynchronous jobs and recurring tasks with Symfony Messenger and Scheduler
---

Symfony has two answers to "run this outside a web request": **Messenger** for asynchronous jobs, **Scheduler** for recurring tasks. FrankenDeploy runs the process both of them need, a Messenger worker, as a container next to your app.

## Enabling the worker

`frankendeploy init` sets it up when it finds `config/packages/messenger.yaml`:

```yaml
messenger:
  enabled: true
  transports:
    - async
    - scheduler_default
```

The transports are read from your `messenger.yaml` (with `%env()%` DSNs resolved through `.env` and `.env.local`); the failure transport and `sync://` ones are left out. `scheduler_default` appears when `symfony/scheduler` is installed. `init` announces every choice, and you can edit the list.

The transport DSN itself (`MESSENGER_TRANSPORT_DSN`) is a production secret like any other:

```bash
frankendeploy env set prod MESSENGER_TRANSPORT_DSN --from-stdin
```

`doctrine://default` needs nothing else. An `amqp://` or `redis://` DSN gets the matching PHP extension added by `init`; the AMQP or Redis server itself is yours to provide (a managed one at your provider, or a container you run on the server).

## What runs

Each deploy starts one container, `<app>-worker`, from the **same image** as the app, with the same `.env.local` and the same `DATABASE_URL`:

```
php bin/console messenger:consume async scheduler_default --time-limit=3600 --memory-limit=256M -vv
```

- `--time-limit=3600` and `--memory-limit=256M` make the worker exit cleanly every hour or when it grows; Docker's `restart: unless-stopped` starts it again. That is the recommended way to run a long-lived PHP process: no leak survives an hour.
- The worker is stopped and recreated **after** the traffic switch of a deploy, so it never runs code older than the app. `rollback` does the same with the rolled-back image.
- `pcntl` is added to the PHP extensions by `init`, so the worker finishes its current message before stopping (graceful shutdown).
- One worker container. For more consumers, that is a future option; today, scale the message handling inside the app (batching) before scaling processes.

```bash
frankendeploy logs prod --service worker        # its logs
frankendeploy logs prod --service all -f        # app and worker together
```

## Scheduled tasks (cron)

With [Symfony Scheduler](https://symfony.com/doc/current/scheduler.html), a recurring task is a message emitted on a schedule and handled by the worker:

```php
#[AsCronTask('0 3 * * *')]
final class PurgeOldSessions
{
    public function __invoke(): void { /* ... */ }
}
```

The `scheduler_default` transport is consumed by the worker FrankenDeploy runs, so the task runs at 3:00 on the server, restarts with each deploy, and its failures land in the failure transport like any message. No crontab on the host, nothing to configure on the server.

If you would rather keep a classic cron, the host can call the app's console:

```
# /etc/cron.d/my-app on the server
0 3 * * * root docker exec my-app php bin/console app:purge-sessions
```

It works, but it is outside FrankenDeploy: it does not follow deploys and rollbacks, and it is yours to maintain.

## Failures and retries

Messenger's retry and failure transport work as usual. The messages that failed for good sit in the failure transport:

```bash
frankendeploy exec prod php bin/console messenger:failed:show
frankendeploy exec prod php bin/console messenger:failed:retry
```

## Without a worker

Leave `messenger.enabled: false`: no container is started. Scheduled tasks then have nothing to run them; use the host cron above, or enable the worker.
