# Changelog

All notable changes to FrankenDeploy will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.5.0] - 2026-01-31

### Added

- **Init Domain Flag**: Add `--domain` flag to `frankendeploy init` to set deploy domain during initialization (#11) - @yoanbernabeu
- **Migration Warning**: Warn when migrations directory is empty but entities exist (#10) - @yoanbernabeu

### Fixed

- **Shared Dirs Permissions**: Correct shared_dirs permissions for container user 1000:1000 (#9) - @yoanbernabeu
- **SQLite Handling**: Improve SQLite database handling (#8) - @yoanbernabeu

## [0.4.0] - 2026-01-31

Initial public release with core deployment features.

[Unreleased]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/yoanbernabeu/frankendeploy/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/yoanbernabeu/frankendeploy/releases/tag/v0.4.0
