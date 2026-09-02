---
title: CI/CD
description: Deploy from GitHub Actions, GitLab CI, or any runner
---

FrankenDeploy is a single binary that needs an SSH key and a server entry. Any CI runner can deploy; the recipes below are complete and tested.

## The pieces

| Piece | How |
|---|---|
| The CLI | The install script, in the job (`curl -fsSL …/scripts/install.sh \| sh`) |
| The SSH key | A dedicated, **unencrypted** deploy key, registered on the server, passed as the `FRANKENDEPLOY_SSH_KEY` secret (raw PEM content) |
| The host key | `FRANKENDEPLOY_KNOWN_HOSTS` with the output of `ssh-keyscan 203.0.113.42`, so the runner verifies the server like your machine does |
| The server entry | `frankendeploy server add prod user@host --skip-test` in the job: the global config does not exist on a fresh runner |
| No prompts | `--yes`: every question gets its default; a passphrase, a host key confirmation or an architecture choice that would need a human fails explicitly instead |
| The gate | `frankendeploy doctor prod` exits 1 on a blocking problem: run it before `deploy` |

`FRANKENDEPLOY_SERVER=prod` lets you write `frankendeploy deploy --yes` without the argument.

## GitHub Actions

Secrets to create: `SSH_KEY` (private key), `KNOWN_HOSTS` (`ssh-keyscan 203.0.113.42`), `SSH_TARGET` (`root@203.0.113.42`).

```yaml
name: Deploy

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install FrankenDeploy
        run: curl -fsSL https://raw.githubusercontent.com/yoanbernabeu/frankendeploy/main/scripts/install.sh | sh

      - name: Deploy
        env:
          FRANKENDEPLOY_SERVER: prod
          FRANKENDEPLOY_SSH_KEY: ${{ secrets.SSH_KEY }}
          FRANKENDEPLOY_KNOWN_HOSTS: ${{ secrets.KNOWN_HOSTS }}
        run: |
          frankendeploy server add prod ${{ secrets.SSH_TARGET }} --skip-test
          frankendeploy doctor prod
          frankendeploy deploy --yes
```

The runner is x86_64, like most VPS, so the image is built on the runner and transferred. On an arm64 runner or for an arm64 server, add `--remote-build`.

## GitLab CI

```yaml
deploy:
  stage: deploy
  image: ubuntu:24.04
  before_script:
    - apt-get update && apt-get install -y curl ca-certificates
    - curl -fsSL https://raw.githubusercontent.com/yoanbernabeu/frankendeploy/main/scripts/install.sh | sh
  script:
    - frankendeploy server add prod $SSH_TARGET --skip-test
    - frankendeploy doctor prod
    - frankendeploy deploy --yes
  variables:
    FRANKENDEPLOY_SERVER: prod
    FRANKENDEPLOY_SSH_KEY: $SSH_PRIVATE_KEY
    FRANKENDEPLOY_KNOWN_HOSTS: $KNOWN_HOSTS
  only:
    - main
```

Define `SSH_PRIVATE_KEY`, `KNOWN_HOSTS` and `SSH_TARGET` as masked, protected CI variables. Docker is needed on the runner for a local build (`image: ubuntu:24.04` with a Docker socket, or a `docker:dind` service); with `--remote-build`, the runner needs nothing but the CLI.

## Environment variables in CI

Secrets stay on the server; the pipeline does not need them. To set one from CI (a rotated API key, for instance):

```bash
printf '%s' "$NEW_API_KEY" | frankendeploy env set prod API_KEY --from-stdin --yes
```

## Staging then production

Two server entries, two jobs, or one job with a variable:

```bash
frankendeploy server add $TARGET $SSH_TARGET --skip-test
frankendeploy deploy $TARGET --yes
```

with `TARGET=staging` on merge requests and `TARGET=prod` on `main`.

## What `--yes` changes

- Host key: a server not in `FRANKENDEPLOY_KNOWN_HOSTS` is refused (no interactive confirmation)
- SSH keys: passphrase-protected keys are skipped
- Architecture mismatch: refused unless `--remote-build` or `remote_build: true` on the server
- Env pre-flight: a missing `APP_SECRET` fails the job (no generation prompt)
- `app remove`: `--yes` counts as the confirmation, `--force` is not needed

Everything else behaves as on your machine. `--verbose` prints every remote command with secrets masked, which is what to attach to an issue if something goes wrong.

## Reference

| Variable | Description |
|----------|-------------|
| `FRANKENDEPLOY_SERVER` | Server name to use when the argument is omitted |
| `FRANKENDEPLOY_SSH_KEY` | Private key content (raw PEM, unencrypted) |
| `FRANKENDEPLOY_KNOWN_HOSTS` | `known_hosts` content with the server's host key |
| `FRANKENDEPLOY_SKIP_HOST_KEY_CHECK` | Skip host key verification. Not recommended: use `FRANKENDEPLOY_KNOWN_HOSTS` |
