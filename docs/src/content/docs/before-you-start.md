---
title: Before You Start
description: What you need before your first deployment, explained from scratch
---

This page is for you if you have never put an application online on a server. It takes about twenty minutes and ends with everything FrankenDeploy needs. If you already have a VPS, an SSH key and a domain, jump to [Your First Deployment](/frankendeploy/first-deployment/).

## What you will need

| | What | Why |
|---|---|---|
| 1 | A Symfony application that runs on your machine | It is what we deploy |
| 2 | A **VPS** (a rented Linux server) running Ubuntu or Debian | It is where the app will live |
| 3 | An **SSH key** on your machine, registered on the VPS | It is how FrankenDeploy talks to the server |
| 4 | A **domain name** pointing to the VPS | It is your `https://` address |
| 5 | FrankenDeploy installed on your machine | See [Installation](/frankendeploy/installation/) |

Docker on your machine is **optional**: FrankenDeploy can build on the server. You only need it for the local development environment or local builds.

## 1. A VPS

A VPS is a small virtual machine rented by the month, with a public IP address, on which you have full control. Any provider works. What to pick when ordering:

- **Operating system**: Ubuntu 24.04 or Debian 12. FrankenDeploy refuses anything else, on purpose.
- **Size**: 2 vCPU and 2 GB of RAM are comfortable for a Symfony app with its database; 1 GB works for a small site. You can resize later.
- **Authentication**: choose **SSH key** if the provider offers it, and paste the public key you create in the next step. If the provider only gives you a root password, that works too for the first connection.

Write down the **public IP address** the provider gives you, for example `203.0.113.42`. The docs use that address everywhere.

## 2. An SSH key

SSH is the encrypted connection between your machine and the server. A key pair replaces the password: the private key stays on your machine, the public key goes on the server.

If you already have `~/.ssh/id_ed25519`, skip to the test. Otherwise, on macOS or Linux (on Windows, use the same commands in PowerShell or WSL):

```bash
ssh-keygen -t ed25519 -C "you@example.com"
```

Accept the default location. A passphrase is a good idea; FrankenDeploy asks for it when needed and never stores it.

Register the public key on the server. If the provider let you paste it at creation, it is already there. Otherwise, with the root password the provider gave you:

```bash
ssh-copy-id root@203.0.113.42
```

Then test:

```bash
ssh root@203.0.113.42
```

The first time, SSH shows the server's fingerprint and asks you to confirm: answer `yes`. You should land on a prompt on the server. Type `exit` to come back.

<div class="callout callout-tip">

**root or a dedicated user?**

Connecting as `root` is the simplest and works fine with FrankenDeploy. Providers sometimes create a user such as `ubuntu` or `debian` with passwordless `sudo` instead: that works too. What does not work is a user that has to type a password for `sudo`.

</div>

## 3. A domain name

Your visitors will not type an IP address. Buy a domain (or use a subdomain of one you own), then create a **DNS record of type A** pointing to the server's IP:

| Type | Name | Value |
|------|------|-------|
| A | `my-app.com` (or `app.my-domain.com`) | `203.0.113.42` |

Where: in the DNS zone of the registrar where you bought the domain. The record can take from a few minutes to a few hours to propagate. You can check from your machine:

```bash
dig +short my-app.com
# 203.0.113.42   ← done
```

FrankenDeploy checks it too (`frankendeploy doctor`) and refuses to deploy while the domain does not point to the server: without that, the HTTPS certificate cannot be issued.

You can deploy without a domain, the app will just not be reachable from the Internet yet. Add the domain later in `frankendeploy.yaml`.

## 4. The Symfony application

Any Symfony application works, from a fresh `symfony new` to a large project. FrankenDeploy reads `composer.json`, `.env`, `config/packages/` to configure itself. Three things worth checking before the first deploy:

- The application starts with `APP_ENV=prod` on your machine (`APP_ENV=prod php bin/console cache:clear` shows configuration errors early).
- If you use Doctrine, your migrations are generated and committed (`php bin/console make:migration`). FrankenDeploy runs them at each deploy and warns when entities exist without migrations.
- Your `.env` only contains defaults, never production secrets: those go on the server with `frankendeploy env set`.

## 5. FrankenDeploy

Follow [Installation](/frankendeploy/installation/), then check:

```bash
frankendeploy --version
```

## Ready

You have an IP, a working `ssh root@203.0.113.42`, a domain pointing to it, and the CLI installed. [Your First Deployment](/frankendeploy/first-deployment/) takes it from here.

## Vocabulary you will meet

**VPS**, **SSH**, **DNS**, **reverse proxy**, **container**, **image**, **health check**, **migration**, **blue-green**: each one is defined in a few words in the [Glossary](/frankendeploy/glossary/).
