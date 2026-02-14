# Changelog

All notable changes to FrankenDeploy will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.8.0...HEAD
[0.8.0]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/yoanbernabeu/frankendeploy/releases/tag/v0.4.0
