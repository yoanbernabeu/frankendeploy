---
title: Installation
description: How to install FrankenDeploy on your system
---

## Quick Install

The fastest way to install FrankenDeploy:

```bash
curl -fsSL https://raw.githubusercontent.com/yoanbernabeu/frankendeploy/main/scripts/install.sh | sh
```

This script automatically detects your OS and architecture and installs the latest version.

## Using Go

If you have Go 1.21+ installed:

```bash
go install github.com/yoanbernabeu/frankendeploy/cmd/frankendeploy@latest
```

## Homebrew (macOS)

```bash
brew tap yoanbernabeu/tap
brew install frankendeploy
```

## Manual Download

Download the binary for your platform from the [GitHub Releases](https://github.com/yoanbernabeu/frankendeploy/releases) page.

### Linux (amd64)
```bash
curl -LO https://github.com/yoanbernabeu/frankendeploy/releases/latest/download/frankendeploy_linux_amd64.tar.gz
tar -xzf frankendeploy_linux_amd64.tar.gz
sudo mv frankendeploy /usr/local/bin/
```

### macOS (Apple Silicon)
```bash
curl -LO https://github.com/yoanbernabeu/frankendeploy/releases/latest/download/frankendeploy_darwin_arm64.tar.gz
tar -xzf frankendeploy_darwin_arm64.tar.gz
sudo mv frankendeploy /usr/local/bin/
```

### Windows
Download `frankendeploy_windows_amd64.zip` and add it to your PATH.

## Verify Installation

```bash
frankendeploy --version
```

## Requirements

### For Local Development
- **Docker** - Required for running containers locally
- **Docker Compose** - Usually included with Docker Desktop

### For Deployment
- **SSH access** to your VPS
- **SSH key** for authentication

## Next Steps

- [Quick Start](/frankendeploy/quickstart/) - Deploy your first Symfony app
