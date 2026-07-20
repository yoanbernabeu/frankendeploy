# Changelog

All notable changes to FrankenDeploy will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.9.0...HEAD
[0.9.0]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.8.1...v0.9.0
[0.8.1]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.8.0...v0.8.1
[0.8.0]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/yoanbernabeu/frankendeploy/releases/tag/v0.4.0
