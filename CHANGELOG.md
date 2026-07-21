# Changelog

All notable changes to FrankenDeploy will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.12.0] - 2026-07-21

This release makes long-running servers sustainable: the database gets a safety net before every migration, and neither Docker images nor container logs can fill the VPS disk anymore. All changes were validated live on a real VPS (managed PostgreSQL, real Doctrine migrations).

### Added

- **Automatic Database Backup Before Migrations**: with a managed database, any `pre_deploy` hook containing `doctrine:migrations:migrate` first dumps the database (`pg_dump` / `mysqldump --single-transaction`) gzipped into `shared/backups/`, chmod 600, verified non-empty, with retention aligned on `keep_releases`; a failed backup aborts the deploy (#81) - @yoanbernabeu
- **Honest Post-Migration Rollback**: when a rollback happens after the migration ran (failed health check, swap, or partial hook), the CLI now states explicitly that the schema was NOT rolled back with the code, prints the backup path, a restore one-liner, and the backward-compatible-migrations tip (#81) - @yoanbernabeu
- **Docker Image Pruning**: images whose tag left the `keep_releases` window are removed after each deploy (never forced: an image still used by a container survives); dangling layers are pruned after remote builds — previously ~1 GB per deploy accumulated forever, filling a 20-40 GB VPS in 10-30 deploys (#82) - @yoanbernabeu
- **Log Rotation Everywhere**: every container FrankenDeploy starts or generates (app, worker, managed database, Caddy, compose services) caps its json-file logs at 10 MB × 3 files (#83) - @yoanbernabeu
- **Resource Limits**: optional `deploy.memory_limit` / `deploy.cpu_limit` applied to the app container in both compose-prod and the deploy `docker run`, with strict format validation (#83) - @yoanbernabeu

### Fixed

- **Leftover Image Tar**: with local builds, the image tar on the server is now removed even when `docker load` fails (previously 500 MB+ per failed deploy stayed in `/tmp`) (#82) - @yoanbernabeu
- **Dev SERVER_NAME**: the dev compose no longer defaults `SERVER_NAME` to the production domain, which triggered real Let's Encrypt issuance attempts from the local machine (#83) - @yoanbernabeu
- **Dev Stack Refresh**: archived `mailhog/mailhog` (no arm64 image) replaced by `axllent/mailpit`; EOL `rabbitmq:3` bumped to `rabbitmq:4` (#83) - @yoanbernabeu

### Security

- **Dev Database Exposure**: dev compose database ports now bind to `127.0.0.1` instead of every interface (#83) - @yoanbernabeu
- **MySQL Root Password**: the prod compose no longer reuses the app password as the MySQL root password (`MYSQL_RANDOM_ROOT_PASSWORD=1`) (#83) - @yoanbernabeu

## [0.11.0] - 2026-07-21

This release closes the second P1 wave of the production-readiness audit, focused on security and robustness: SSH authentication that matches OpenSSH expectations, a production Docker image that fails at build time instead of at the first request, and secrets that never end up world-readable. All changes were validated live on a real VPS.

### Added

- **SSH Agent & Encrypted Keys**: Authentication now tries ssh-agent first (`SSH_AUTH_SOCK`), then the key file; passphrase-protected keys are fully supported with a hidden prompt (asked at most once, skipped when the agent already holds the key), and `server add` no longer silently skips them (#78) - @yoanbernabeu
- **Trust-On-First-Use**: The first connection to a new server shows the host key SHA256 fingerprint with an OpenSSH-style confirmation and records it in `known_hosts` — no more manual `ssh` required before using FrankenDeploy (#78) - @yoanbernabeu
- **Secrets via stdin**: `env set <server> KEY --from-stdin` reads the value from a hidden prompt or a pipe (`openssl rand -hex 32 | frankendeploy env set prod APP_SECRET --from-stdin`), keeping secrets out of shell history (#80) - @yoanbernabeu
- **Configurable Node Version**: The asset build stage uses `node:22-slim` by default (Node 20 EOL April 2026), configurable via `assets.node_version` (#79) - @yoanbernabeu

### Fixed

- **Host Key Errors**: A changed server key (reinstalled VPS — or a MITM) now fails immediately with the exact `ssh-keygen -R <host>` command instead of 3 retries burying a cryptic error; authentication failures are no longer retried either (#78) - @yoanbernabeu
- **Silent Build Failures**: The production image ran `composer post-install-cmd || true`, masking any failure until the first HTTP request in production; scripts now fail the build, followed by an explicit `cache:clear` + `cache:warmup` (#79) - @yoanbernabeu
- **Production OPcache**: The image kept dev-oriented defaults (`validate_timestamps=1`, re-stat on every request); the prod stage now ships tuned settings and `opcache.preload` when `config/preload.php` exists (#79) - @yoanbernabeu
- **Image Size**: A whole-tree `chown -R` layer duplicated every file in the image; ownership is now set at `COPY --chown` time (#79) - @yoanbernabeu
- **App-Level Health Check**: The container `HEALTHCHECK` probed the Caddy admin endpoint (proving only that Caddy was up); it now probes the application on `healthcheck_path`, making Docker health and `env --reload` waits meaningful (#79) - @yoanbernabeu
- **Env File Corruption**: Values containing double quotes corrupted `.env.local` on write, and unsorted writes reordered the file every time; content is now sorted with proper escaping and lossless round-trip (#80) - @yoanbernabeu

### Security

- **World-Readable Secrets**: `env set`/`push`/`remove` left `.env.local` with the server umask (typically 644); every env write now goes through a single writer enforcing `chmod 600` and container-user ownership (#80) - @yoanbernabeu
- **Secrets in Logs**: Verbose command logging only masked 4 hardcoded patterns — `MAILER_DSN`, `*_TOKEN`, `*_API_KEY` and others were logged in clear; masking now covers any assignment whose key matches the shared sensitive-key list, also used by `env list` (#80) - @yoanbernabeu
- **Composer Superuser Flag**: `COMPOSER_ALLOW_SUPERUSER=1` no longer persists as an ENV in the final image (#79) - @yoanbernabeu

## [0.10.0] - 2026-07-20

This release closes the "first contact" wave of the production-readiness audit: every fix targets a situation where a first-time user was left blocked or misled. All changes were validated live on a real VPS.

### Added

- **Self-Sufficient Deploy**: `deploy` generates missing Docker artifacts (`Dockerfile`, `docker-entrypoint.sh`, `.dockerignore`) automatically — the novice flow `init` → `deploy` no longer dies on a cryptic "open Dockerfile: no such file or directory"; customized files are never overwritten (#73) - @yoanbernabeu
- **Health Check Tuning**: New `deploy.healthcheck_timeout` / `healthcheck_retries` / `healthcheck_interval` YAML settings, and new `--skip-env-check` / `--skip-healthcheck` deploy flags (#76) - @yoanbernabeu

### Fixed

- **Cross-Architecture Build**: The local Docker build hardcoded `--platform linux/amd64`; an Apple Silicon Mac deploying to an ARM VPS (Hetzner CAX, AWS Graviton) produced an unusable image surfacing as an unexplained health check failure. The platform now follows the detected server architecture (#75) - @yoanbernabeu
- **Health Check UX**: On failure, the last 50 log lines of the failing container are printed before rollback removes it; the default window grew from ~15s to 90s (a cold Symfony container needs opcache warmup and DB wait); `--force` no longer prints "Health check passed" after a failure; SSH drops during checks no longer crash the CLI (#76) - @yoanbernabeu
- **Honest First Deploy**: A Caddy configuration failure (container stopped, reload error) on the app's first public exposure now fails the deploy with an explicit "NOT publicly reachable" error instead of printing "Deployment complete!" with an unreachable URL; the caddy container is checked before the reload (#77) - @yoanbernabeu

### Documentation

- **Docs Site Sync**: The static site had not been updated since v0.6.0 and documented pre-blue-green behavior, broken download URLs (404), PHP 8.1 support, and a nonexistent worker mode; every page now matches the actual behavior (#74) - @yoanbernabeu

## [0.9.0] - 2026-07-20

This release closes every P0 finding from the production-readiness audit. All fixes were validated live on a real VPS deployment (API Platform app, managed PostgreSQL, SSH gateway, Let's Encrypt).

### Added

- **API Platform Auto-Detection**: Detect `api-platform/core` / `api-platform/symfony` and default the healthcheck path to `/api` — a pure API returns 404 on `/`, which previously caused guaranteed rollbacks of healthy deployments (#65) - @yoanbernabeu

### Fixed

- **Scanner Panic**: `init` crashed (SIGSEGV) on projects without `package.json`, such as an API Platform skeleton (#63) - @yoanbernabeu
- **PHP Version Floor**: A stock Symfony 6.4 skeleton (`php: >=8.1`) generated the nonexistent `frankenphp:1-php8.1` image; versions are now floored at 8.2 with an explicit warning, and exclusive upper bounds (`<8.4`) are respected (#64) - @yoanbernabeu
- **Entrypoint Crash-Loop**: `DATABASE_URL` without an explicit port (the standard managed-DB case) crash-looped the container; host/port are now parsed with POSIX expansion, covering all Doctrine schemes and passwords containing `@` (#66) - @yoanbernabeu
- **Caddy Health Check**: The generated app config hardcoded `health_uri /`, marking API-only upstreams unhealthy and turning every request into a 503; it now follows `deploy.healthcheck_path` (#69) - @yoanbernabeu
- **Zero-Downtime Swap**: The container swap did stop → rm → rename, guaranteeing a downtime window and leaving the site down permanently on a failed rename; the swap is now rename-based (measured live: 0 dropped requests) with automatic restore of the old container on failure (#71) - @yoanbernabeu
- **Rollback Parity**: Rollback lost shared dirs and the managed `DATABASE_URL`, swallowed errors, had no health check and could select a release newer than the current one; it now reuses the full deploy pipeline (same mounts/env, health check before swap, zero-downtime handover, worker rollback), and `env --reload` was unified on the same primitives (#72) - @yoanbernabeu

### Security

- **Shell Injection via shared_files**: `shared_files` entries were interpolated raw into remote shell commands without validation (#67) - @yoanbernabeu
- **SSH Lockout Prevention**: `server setup` opened hardcoded port 22 in UFW before enabling it, locking users out of servers with a custom SSH port; the configured port and the actual sshd session port are now both allowed before the firewall goes up (#68) - @yoanbernabeu

## [0.8.1] - 2026-07-20

### Fixed

- **Generator Validation**: Close validation gaps for heredoc injection, env keys, and Docker image tags (#30) - @yoanbernabeu

### Changed

- **Deploy Connection**: Use `ConnectToServer()` instead of manual connection setup (#31) - @yoanbernabeu
- **Constants**: Replace hardcoded paths and ports with constants (#32) - @yoanbernabeu
- **Managed Databases**: Use `dbDriverRegistry` in `deployManagedDatabase` (#33) - @yoanbernabeu
- **Health Checks**: Wire `HealthChecker` into the deploy path (#34) - @yoanbernabeu
- **SSH Testability**: Accept `ssh.Executor` interface instead of `*ssh.Client` in deploy (#35) - @yoanbernabeu
- **Dead Code Cleanup**: Remove dead code and fix env/key detection quirks (#36) - @yoanbernabeu
- **Validation Unification**: Unify validation and remove dead code across scanner/config/generator (#37) - @yoanbernabeu
- **Template Data Flow**: Close template data-flow gaps (#38) - @yoanbernabeu

## [0.8.0] - 2026-02-14

### Changed

- **SSH Executor Interface**: Add `ssh.Executor` interface for dependency injection and testability across all SSH-dependent code (#20) - @yoanbernabeu
- **Context Propagation**: Add `context.Context` to all `Exec`/`ExecStream` calls (~130 call sites) enabling cancellation and timeout (#20) - @yoanbernabeu
- **Error Wrapping**: Replace 15 instances of `%s` error formatting with proper `%w` wrapping using new `CommandError` type and `ExecResult.Err()` helper (#20) - @yoanbernabeu
- **Code Deduplication**: Extract `stopAndRemoveContainer`/`forceRemoveContainer` helpers, `EffectiveSharedDirs()`/`EffectiveSharedFiles()` config methods, and merge `ConnectToServer` internals (#20) - @yoanbernabeu

### Fixed

- **Critical ExitError Bug**: Fix `ExitError` type assertion that silently swallowed all non-zero exit codes — `ExitCode` was always 0, making all exit code checks dead code (#20) - @yoanbernabeu
- **Security**: Remove `StrictHostKeyChecking=no` from scp and rsync commands (#20) - @yoanbernabeu

### Removed

- **Dead SSH Code**: Remove 14 unused SSH methods including `transfer.go`, `ExecMultiple`, `Shell`, `StreamOutput`, `IsConnected`, `GetClient` (#20) - @yoanbernabeu

## [0.7.0] - 2026-02-14

### Added

- **Generator Hardening**: Add validation, constants, DB registry, and thread-safe template loading (#13) - @yoanbernabeu

### Fixed

- **Scanner & Config Validation**: Fix silent PostgreSQL fallback, add validation for extensions/domain/healthcheck/messenger, YAML strict mode, .env inline comment parsing, config file permissions (#17) - @yoanbernabeu
- **Security Hardening**: Prevent command injection in shell commands and fix ineffectual assignment in password masking - @yoanbernabeu
- **Blue-Green Deployment**: Add zero-downtime deployment with rollback, SSH reconnection with exponential backoff, deploy state machine (#16) - @yoanbernabeu
- **Compose Generator**: Fix 10 Docker Compose generator bugs including database URL building, SQLite handling, and YAML escaping (#18) - @yoanbernabeu

## [0.6.0] - 2026-01-31

### Added

- **Cross-Architecture Detection**: Auto-detect architecture mismatch between local machine and server, suggest remote build (#12) - @yoanbernabeu
- **Server Set Command**: Add `server set` command to configure server-specific options like `remote_build` (#12) - @yoanbernabeu
- **No Remote Build Flag**: Add `--no-remote-build` flag to force local build (#12) - @yoanbernabeu

## [0.5.0] - 2026-01-31

### Added

- **Init Domain Flag**: Add `--domain` flag to `frankendeploy init` to set deploy domain during initialization (#11) - @yoanbernabeu
- **Migration Warning**: Warn when migrations directory is empty but entities exist (#10) - @yoanbernabeu

### Fixed

- **Shared Dirs Permissions**: Correct shared_dirs permissions for container user 1000:1000 (#9) - @yoanbernabeu
- **SQLite Handling**: Improve SQLite database handling (#8) - @yoanbernabeu

## [0.4.0] - 2026-01-31

Initial public release with core deployment features.

[Unreleased]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.12.0...HEAD
[0.12.0]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.9.0...v0.10.0
[0.9.0]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.8.1...v0.9.0
[0.8.1]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.8.0...v0.8.1
[0.8.0]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/yoanbernabeu/frankendeploy/releases/tag/v0.4.0
