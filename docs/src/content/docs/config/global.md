---
title: Global Configuration
description: Where servers are declared, and how to manage them
---

## Location

The global configuration stores your servers. It lives **outside your projects**, in the user configuration directory of your system:

| System | Path |
|--------|------|
| macOS | `~/Library/Application Support/frankendeploy/config.yaml` |
| Linux | `~/.config/frankendeploy/config.yaml` (or `$XDG_CONFIG_HOME/frankendeploy/config.yaml`) |
| Windows | `%AppData%\frankendeploy\config.yaml` |

It is created by the first `frankendeploy server add`. In CI, declare the server in the job with `frankendeploy server add ... --skip-test`.

Two files, two roles: `frankendeploy.yaml` in the project describes the **application** and is versioned; the global config describes your **machines** and never leaves your computer.

## Structure

```yaml
# Defaults for new servers
default_user: deploy
default_port: 22

# SSH connection timeout in seconds (optional, default: 30)
ssh_timeout: 30

servers:
  prod:
    host: 203.0.113.42
    user: deploy
    port: 22
    key_path: ~/.ssh/id_ed25519   # optional, see SSH authentication
    remote_build: true            # build images on the server

  staging:
    host: staging.example.com
    user: deploy
    port: 2222
```

### Server Fields

| Field | Description | Default |
|-------|-------------|---------|
| `host` | Hostname or IP | Required |
| `user` | SSH user | Required |
| `port` | SSH port | 22 |
| `key_path` | SSH private key. When absent, ssh-agent and the usual keys are tried | Auto |
| `remote_build` | Build Docker images on the server instead of locally | Unset (asked on architecture mismatch) |

## Adding Servers

```bash
frankendeploy server add prod deploy@203.0.113.42
```

FrankenDeploy tests the SSH connection right away. On the first connection it shows the host key fingerprint and asks for confirmation, like OpenSSH, then records it in `~/.ssh/known_hosts`. If the connection fails, it looks for keys in `~/.ssh/`:
- **Interactive**: lists the keys found (passphrase-protected ones included, the passphrase is prompted) and lets you choose
- **`--yes` (CI)**: tries unencrypted keys one after the other

The working key is saved as `key_path`.

```bash
frankendeploy server add staging deploy@staging.example.com --port 2222 --key ~/.ssh/staging_key
frankendeploy server add ci user@host --skip-test     # no connection test
```

## SSH Authentication

At connection time, FrankenDeploy tries in order:

1. **ssh-agent**, when `SSH_AUTH_SOCK` is set
2. **A key file**: `key_path`, or `~/.ssh/id_ed25519`, then `~/.ssh/id_rsa`, then the other keys in `~/.ssh/`

Passphrase-protected keys are supported: the passphrase is prompted when needed and never stored. In non-interactive mode (`--yes`, CI), an encrypted key cannot be prompted: use ssh-agent or `FRANKENDEPLOY_SSH_KEY` (see [CI/CD](/frankendeploy/guides/ci-cd/)).

If the host key of a server changes (reinstalled VPS), the connection is refused with a clear message. When the change is expected: `ssh-keygen -R <host>`.

## Server Options

```bash
frankendeploy server set prod remote_build true
frankendeploy server set prod remote_build false
```

`remote_build` is the only key today. FrankenDeploy also sets it for you when you accept the remote build proposal on an architecture mismatch.

## Managing Servers

```bash
frankendeploy server list             # every server, with its options
frankendeploy server status prod      # Docker, Caddy, network, CPU, memory, disk, per-app usage
frankendeploy server remove staging   # forget the server (nothing changes on the machine)
```

## Several Projects, Several Servers

Servers are shared by all your projects: deploy two applications to `prod` from two project directories, they get their own directory, database, network and Caddy configuration on the server. `frankendeploy app list prod` shows what runs there.

## Manual Editing

The file is plain YAML. To add a server without testing the connection:

```yaml
servers:
  prod:
    host: my-vps.com
    user: deploy
    port: 22
    key_path: ~/.ssh/id_ed25519
```

## Troubleshooting

### SSH Connection Issues

`frankendeploy doctor prod` reports the SSH connection first. To test by hand:

```bash
ssh -i ~/.ssh/id_ed25519 -p 22 deploy@your-server.com
```

Key permissions matter: `chmod 600 ~/.ssh/id_ed25519` and `chmod 700 ~/.ssh`.

### Behind a Gateway or a Bastion

Declare the gateway as the host and its port as `port`. `server setup` reads the port the SSH daemon actually uses on the machine and opens it in the firewall too, so a gateway on 3022 in front of an sshd on 22 never locks you out.
