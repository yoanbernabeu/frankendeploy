---
title: Global Configuration
description: Configure FrankenDeploy globally
---

## Location

Global configuration is stored at:

```
~/.config/frankendeploy/config.yaml
```

This file stores server configurations that are shared across all your projects.

## Structure

```yaml
# Default settings for new servers
default_user: deploy
default_port: 22

# Configured servers
servers:
  production:
    host: prod.example.com
    user: deploy
    port: 22
    key_path: ~/.ssh/id_ed25519
    remote_build: true  # Build images on server (for cross-architecture)
    apps:
      my-app: /opt/frankendeploy/apps/my-app

  staging:
    host: staging.example.com
    user: deploy
    port: 22
```

## Server Configuration

### Adding Servers

```bash
frankendeploy server add production deploy@prod.example.com
```

FrankenDeploy automatically tests the SSH connection after adding a server. If the default key doesn't work, it will:
- In interactive mode: show available keys and let you choose
- In CI/CD mode (`--yes`): try keys automatically

With all options:
```bash
frankendeploy server add staging deploy@staging.example.com \
  --port 2222 \
  --key ~/.ssh/id_rsa \
  --skip-test
```

Use `--skip-test` to skip the automatic SSH connection test.

### Server Fields

| Field | Description | Default |
|-------|-------------|---------|
| `host` | Server hostname or IP | Required |
| `user` | SSH username | Required |
| `port` | SSH port | 22 |
| `key_path` | Path to SSH private key | Auto-detected |
| `remote_build` | Build Docker images on server instead of locally | Auto-detected |
| `apps` | Deployed applications | Auto-populated |

### Configuring Server Options

Use `frankendeploy server set` to configure server-specific options:

```bash
# Enable remote build for a server
frankendeploy server set production remote_build true

# Disable remote build
frankendeploy server set production remote_build false
```

### Managing Servers

List all servers:
```bash
frankendeploy server list
```

Check server status:
```bash
frankendeploy server status production
```

Remove a server:
```bash
frankendeploy server remove staging
```

## SSH Key Auto-detection

When adding a server, FrankenDeploy tests the SSH connection. If the connection fails, it discovers available keys in `~/.ssh/` and tries them in order of preference:

1. `~/.ssh/id_ed25519` (preferred)
2. `~/.ssh/id_rsa`
3. Other `id_*` or `*.pem` files

The first working key is automatically saved to the configuration.

**Note:** Passphrase-protected keys are skipped during auto-detection.

## Multiple Projects

The global config is shared across all your projects. When you deploy an app, it's automatically registered under the server's `apps` section.

This allows you to:
- Deploy multiple apps to the same server
- List all apps on a server: `frankendeploy app list production`
- Manage apps independently

## Manual Editing

You can manually edit the config file:

```bash
# Open in your editor
$EDITOR ~/.config/frankendeploy/config.yaml
```

Example for adding a server manually:

```yaml
servers:
  production:
    host: my-vps.com
    user: deploy
    port: 22
    key_path: ~/.ssh/id_ed25519
```

## Troubleshooting

### Config Not Found

If FrankenDeploy can't find the config:
```bash
mkdir -p ~/.config/frankendeploy
touch ~/.config/frankendeploy/config.yaml
```

### SSH Connection Issues

Test SSH connection manually:
```bash
ssh -i ~/.ssh/id_ed25519 deploy@your-server.com
```

Check key permissions:
```bash
chmod 600 ~/.ssh/id_ed25519
chmod 700 ~/.ssh
```
