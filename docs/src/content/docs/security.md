---
title: Security Model
description: What FrankenDeploy protects, how, and what it deliberately leaves to you
---

FrankenDeploy prepares a server and runs your application on it with sane defaults. It is not a hardening tool, and this page is meant to be precise about the boundary: what is handled, with which mechanism, and what remains your responsibility.

## Threat model

The server is a single VPS you own, reachable from the Internet. The risks FrankenDeploy addresses, in order of likelihood:

1. **Automated attacks on the server itself**: SSH brute force, scans of open ports
2. **A compromised application**: a vulnerability in your code or a dependency giving an attacker a shell inside the container
3. **Another application on the same server** reaching yours, or your database
4. **Secrets leaking**: through Git, shell history, world-readable files, logs
5. **Losing data**: a migration that goes wrong, a deploy that breaks the site

It does not address a compromised server (root access obtained by other means), a compromised provider, or attacks on the application logic itself.

## What `server setup` does

| Measure | Mechanism | Verify on the server |
|---|---|---|
| Firewall | UFW, default deny incoming; allowed: your SSH port(s), 80, 443 | `sudo ufw status` |
| SSH brute force | Fail2ban, `sshd` jail: 5 failed attempts in 10 minutes, 1 hour ban | `sudo fail2ban-client status sshd` |
| No lock-out | Both the port you connect to and the port `sshd` listens on are allowed before UFW is enabled | the `Firewall:` line of the setup output |
| Proxy admin API | Caddy's admin endpoint listens on `localhost:2019` inside its container; reloads go through `docker exec` | `docker port caddy` shows only 80 and 443 |

## What every deploy does

| Measure | Mechanism |
|---|---|
| No root in containers | The app and worker run as uid 1000 (`--user 1000:1000`); the image's production stage ends with `USER app` |
| Nothing published | The app, worker and database containers publish no port; Caddy is the only way in, on 80 and 443 |
| Network isolation | One Docker network per app; Caddy is attached to it; two apps on one server cannot reach each other's containers, database included |
| TLS | Let's Encrypt certificate obtained and renewed by Caddy; `Strict-Transport-Security max-age=31536000`; `X-Content-Type-Options`, `X-Frame-Options DENY`, `Referrer-Policy`, `Server` header removed |
| Proxy trust | `SYMFONY_TRUSTED_PROXIES` limited to private subnets, so only Caddy can set `X-Forwarded-*`; Symfony sees the real client IP and the HTTPS scheme |
| Secrets at rest | `shared/.env.local` and `.db_credentials` are `chmod 600`, owned by the deploy user, mounted **read-only** in the containers |
| Secrets in transit | Everything goes over SSH (host key verified like OpenSSH, `known_hosts`); `env set --from-stdin` keeps values out of the shell history; sensitive values are masked in `env list` and `--verbose` output |
| Secrets in Git | Nothing secret goes into `frankendeploy.yaml`; `env.prod` is not used in production |
| Database credentials | Generated (24 random hex characters), one user per app, random one-time root password for MySQL/MariaDB, never written to the image |
| Data safety | Dump before every migration, health check before every traffic switch, previous release kept for rollback |
| Disk exhaustion | Log rotation on every container (10 MB × 3), Caddy access logs 10 MB × 5, images and backups pruned with `keep_releases` |
| Input validation | Hook commands, extra Dockerfile instructions, release tags, app and server names are validated against shell injection before reaching the server |

## What it does not do

Say it plainly so you can plan for it.

| Not done | Why | What to do |
|---|---|---|
| **System security updates** | `server setup` runs once; a VPS deployed in January still runs January's `openssl` in September | `sudo apt install unattended-upgrades` on the server, or update it on a schedule |
| **`sshd` hardening** | FrankenDeploy never edits `sshd_config`: cutting your own access on a server it does not own is a worse outcome than leaving the defaults | Once your key works: `PasswordAuthentication no`, and `PermitRootLogin prohibit-password` if you deploy as root |
| **Encryption of secrets at rest** | `.env.local` is readable by root, and by anyone with SSH access; a key to decrypt it would have to sit next to it | Your provider's disk encryption; treat SSH access to the server as access to the secrets |
| **Off-site backups** | The dumps in `shared/backups/` are on the same disk as the database | Copy them elsewhere on a schedule; the volume `my-app-db-data` is the data itself |
| **Container hardening beyond non-root** | No `cap_drop`, `no-new-privileges` or read-only root filesystem yet | Discussed for a future version |
| **Intrusion detection, monitoring, alerting** | Out of scope | Your usual tools; Caddy's JSON access logs are in `/opt/frankendeploy/caddy/logs/` |
| **Application security** | Your code, your dependencies | `composer audit`, Symfony's security advisories |

A `server harden` command covering the first two lines, with an audit in `doctor`, is [being discussed](https://github.com/yoanbernabeu/frankendeploy/issues/95). Opinions welcome.

## Who can do what

| Actor | Can |
|---|---|
| Anyone on the Internet | Reach Caddy on 80/443, and your app through it. Nothing else answers |
| Someone with your SSH key | Everything: deploy, read secrets, `docker exec` into containers. Protect the key with a passphrase; use a dedicated deploy key in CI |
| Your application, if compromised | Read its own `.env.local`, talk to its own database and worker, reach the Internet. Not: other apps on the server, the Docker socket, the host filesystem, root |
| Another application on the same server | Nothing of yours: different network, different directory, different database credentials |

## Reporting a vulnerability

Use "Report a vulnerability" in the [Security tab](https://github.com/yoanbernabeu/frankendeploy/security) of the repository rather than a public issue.
