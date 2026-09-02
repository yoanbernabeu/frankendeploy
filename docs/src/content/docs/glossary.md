---
title: Glossary
description: The words used across the documentation, in a few lines each
---

**Application server** · The program that runs your PHP code and answers HTTP requests. With FrankenDeploy it is FrankenPHP, which bundles Caddy and PHP in one binary.

**Blue-green deployment** · Starting the new version next to the old one, checking it, then switching the traffic. The old version stays up until the switch, so a failure never takes the site down.

**Caddy** · A web server and reverse proxy that obtains HTTPS certificates by itself. FrankenDeploy runs one on the server, in front of every app.

**Container** · An isolated process started from an image, with its own filesystem and network. Your app, its database and its worker each run in one.

**Deploy** · Building the new version, sending it to the server, and switching the traffic to it.

**DNS, A record** · The Domain Name System turns `my-app.com` into an IP address. An A record is the line saying "`my-app.com` is at `203.0.113.42`".

**Docker** · The tool that builds images and runs containers. FrankenDeploy installs it on the server; on your machine it is optional.

**Health check** · A request FrankenDeploy sends to the new container (`/` or `/api` by default) before switching traffic to it. A 200 means the app works; anything else stops the deploy.

**Image** · A self-contained package: PHP, your code, its dependencies, compiled assets. Built once per deploy, run as a container.

**Managed database** · A database container that FrankenDeploy creates, starts, backs up and connects to your app, without any configuration on your side.

**Migration** · A versioned change to the database schema (Doctrine Migrations). FrankenDeploy runs pending migrations in the new container before switching traffic, after dumping the database.

**Release** · One deployed version of the app on the server, identified by its tag (a timestamp by default). Several are kept so you can roll back.

**Reverse proxy** · A server that receives the visitors' requests and passes them to your app. Caddy does it here: it handles the domain, HTTPS and HTTP/2, and the app only sees plain HTTP on a private port.

**Rollback** · Switching the traffic back to a previous release, with the same health check as a deploy.

**Shared files and directories** · What survives from one release to the next: `.env.local`, logs, sessions, uploads. Everything else is replaced at each deploy.

**SSH** · The encrypted connection between your machine and the server. FrankenDeploy uses it for everything; a key pair replaces the password.

**Swap** · The instant where the traffic moves from the old container to the new one. FrankenDeploy does it by renaming the containers, which takes no time and drops no request.

**VPS** · Virtual Private Server: a rented Linux machine with a public IP, on which you are root. Any provider works.

**Worker** · A process that consumes Symfony Messenger queues (asynchronous jobs, scheduled tasks). FrankenDeploy runs one as a separate container when `messenger.enabled` is true.

**Worker mode (FrankenPHP)** · Keeping the Symfony kernel in memory between requests instead of booting it every time. Much faster; requires the application to be stateless.
