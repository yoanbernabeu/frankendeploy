# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

FrankenDeploy is a Go CLI tool for deploying Symfony applications to any VPS using FrankenPHP. It auto-detects project configuration, generates optimized Docker files, and handles the entire deployment pipeline including SSL, health checks, and rollbacks.

**Status:** Experimental (breaking changes may occur between versions)

## Build & Development Commands

```bash
make build          # Build binary to bin/frankendeploy
make test           # Run tests with race detection
make lint           # Run golangci-lint in Docker
make lint-local     # Run golangci-lint locally
make pre-commit     # Run format, vet, lint, and tests
make test-cover     # Generate coverage report (coverage.html)
make build-all      # Cross-compile for Linux/Darwin/Windows
```

**Documentation:**
```bash
make docs-generate  # Regenerate CLI docs from Cobra commands
make docs-dev       # Run Astro dev server for docs
```

## Architecture

```
cmd/
├── frankendeploy/main.go    # CLI entry point
└── gendocs/main.go          # Cobra documentation generator

internal/
├── cmd/           # Cobra commands (init, deploy, server, dev, logs, exec, shell, rollback, env, app, build)
├── config/        # Configuration management (project config, global config, validation, types)
├── scanner/       # Project analysis (Symfony detection, composer parsing, database/assets detection)
├── generator/     # Docker artifact generation (Dockerfile, docker-compose templates)
├── deploy/        # Deployment orchestration (health checks, rolling deployments)
├── ssh/           # SSH operations (client, file transfer, command execution)
├── security/      # Input sanitization
└── caddy/         # Caddy reverse proxy configuration

docs/              # Astro-based documentation site
```

## Key Patterns

- **CLI Framework:** Cobra (spf13/cobra) with automatic doc generation
- **Configuration:** YAML-based (`frankendeploy.yaml` for project, `~/.config/frankendeploy/config.yaml` for global)
- **SSH:** Uses golang.org/x/crypto (no system SSH dependency)
- **Templates:** Go text/template for Dockerfile and docker-compose generation
- **Scanner:** Auto-detects PHP version, extensions, database type, and asset build tools from composer.json

## Testing

Tests are colocated with implementation files (*_test.go). Run a single test:
```bash
go test -v -run TestFunctionName ./internal/config/
```

## Commit Conventions

Follow conventional commits:
```
type(scope): description

Types: feat, fix, docs, style, refactor, test, chore
```

Branch naming: `feature/`, `fix/`, `docs/`, `refactor/`

## CI/CD

- **ci.yml:** Runs tests, lint, and cross-compilation on push/PR
- **release.yml:** GoReleaser builds on git tags (v*)
- **docs.yml:** Rebuilds Astro docs on docs/ or internal/cmd changes
