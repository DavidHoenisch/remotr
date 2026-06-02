# Environment variables reference

## remotr-server

| Variable | Default | Description |
|----------|---------|-------------|
| `REMOTR_LISTEN` | `:8443` | HTTPS listen address |
| `REMOTR_CONFIG_REPO` | `/config-repo` | Absolute path to configuration repository checkout |
| `REMOTR_RELEASE_REF` | `dev` | Static release label when Git sync is inactive |
| `REMOTR_DATABASE_URL` | (unset) | Postgres DSN. Required for production: enroll, admin API, telemetry, dynamic release ref. Without it, an in-memory registry is used (dev only). |
| `REMOTR_TLS_CERT` | `/certs/server.crt` | Server TLS certificate PEM |
| `REMOTR_TLS_KEY` | `/certs/server.key` | Server TLS private key PEM |
| `REMOTR_TLS_CLIENT_CA` | `/certs/ca.crt` | CA for verifying agent and operator client certificates |
| `REMOTR_CA_CERT` | `/certs/ca.crt` | Remotr CA certificate (issues endpoint and operator certs) |
| `REMOTR_CA_KEY` | `/certs/ca.key` | Remotr CA private key |
| `REMOTR_BOOTSTRAP_FILE` | `/var/lib/remotr/bootstrap.token` | One-time operator bootstrap token file |
| `REMOTR_GIT_REMOTE_URL` | (unset) | Git remote URL for sync (HTTPS recommended for PAT) |
| `REMOTR_GIT_TOKEN` | (unset) | GitHub/Git HTTPS personal access token (never stored in git config) |
| `REMOTR_GIT_USERNAME` | `x-access-token` | HTTPS Git username when using `REMOTR_GIT_TOKEN` (GitHub PAT default) |
| `REMOTR_GIT_BRANCH` | `main` | Branch tracked for release ref |
| `REMOTR_GIT_SYNC_POLL_INTERVAL` | `0` (disabled) | Periodic Git sync interval (for example `5m`, `15m`) |
| `REMOTR_GIT_WEBHOOK_SECRET` | (unset) | Validates `X-Remotr-Git-Webhook-Secret` on webhook POST |

Public CA distribution (no auth): `GET /v1/ca.pem` returns the Remotr CA certificate PEM. Used by `scripts/install-agent.sh` when `REMOTR_CA_*` overrides are unset.

## install-agent.sh

Environment variables for `scripts/install-agent.sh` (see [Installing the agent](../guides/installing-agent.md)):

| Variable | Default | Description |
|----------|---------|-------------|
| `REMOTR_SERVER_URL` | (required) | Server base URL |
| `REMOTR_DEPLOYMENT_TOKEN` | (unset) | Reusable deployment enrollment token |
| `REMOTR_ENROLL_TOKEN` | (unset) | One-time enrollment token |
| `REMOTR_ENROLL_TOKEN_FILE` | (unset) | Path to token file |
| `REMOTR_CA_FINGERPRINT` | (unset) | Optional sha256 fingerprint pin after CA download |
| `REMOTR_CA_FILE` / `REMOTR_CA_PEM` / `REMOTR_CA_URL` | (unset) | Override auto-fetch from `/v1/ca.pem` |
| `REMOTR_VERSION` | `latest` | GitHub release version |
| `REMOTR_YES` | (unset) | Skip install confirmation prompt |

## remotr-agent

| Variable | Default | Description |
|----------|---------|-------------|
| `REMOTR_SERVER_URL` | `https://remotr-server:8443` | Server base URL |
| `REMOTR_SYNC_INTERVAL` | `30s` | Sync poll interval |
| `REMOTR_STATE_DIR` | `/var/lib/remotr` | Enrolled credential directory (`agent.crt`, `agent.key`, `ca.crt`, `state.json`) |
| `REMOTR_ENROLL_TOKEN` | (unset) | One-time enrollment token (enroll subcommand) |
| `REMOTR_ENROLL_TOKEN_FILE` | (unset) | Absolute path to enrollment token file |
| `REMOTR_TLS_CA` | `/certs/ca.crt` | Trust anchor for server TLS |
| `REMOTR_TLS_CERT` | `/certs/agent.crt` | Legacy client cert when not using `REMOTR_STATE_DIR` |
| `REMOTR_TLS_KEY` | `/certs/agent.key` | Legacy client key when not using `REMOTR_STATE_DIR` |
| `REMOTR_COMPOSE_E2E` | (unset) | Compose-only: relax bind-mount permissions for host e2e tests |

Credential resolution order for sync:

1. If `REMOTR_STATE_DIR` contains enrolled credentials → use them
2. Else fall back to `REMOTR_TLS_CERT` / `REMOTR_TLS_KEY` / `REMOTR_TLS_CA`

## remotr (operator CLI)

| Variable | Default | Description |
|----------|---------|-------------|
| `REMOTR_OPERATOR_STATE_DIR` | `~/.config/remotr` | Operator credential directory |
| `REMOTR_CONFIG` | `~/.config/remotr/config.yaml` | Operator CLI config file path |
| `REMOTR_SERVER_URL` | (unset) | Default server URL for admin commands |
| `REMOTR_CA` | (unset) | Remotr CA PEM path for bootstrap |
| `REMOTR_FLEET` | (unset) | Default fleet for `enroll token create` |
| `REMOTR_DATABASE_URL` | (unset) | Used by `remotr init --register-server` |

CLI flags override config file values; config file overrides built-in defaults.

## Docker Compose dev stack

Set in `compose/docker-compose.yml` and seed scripts:

| Variable | Purpose |
|----------|---------|
| `REMOTR_COMPOSE_FLEET` | Fleet name seeded in Postgres (default `test-fleet`) |
| `REMOTR_COMPOSE_DEBIAN_ENROLL_TOKEN` | Fixed token for Debian agent in dev |
| `REMOTR_COMPOSE_ARCH_ENROLL_TOKEN` | Fixed token for Arch agent in dev |

## E2E test overrides

Used by `test/e2e/` when running against Compose:

| Variable | Default | Description |
|----------|---------|-------------|
| `REMOTR_E2E_URL` | `https://localhost:8443` | Server URL |
| `REMOTR_E2E_CA` | `compose/runtime/certs/ca.crt` | CA path |
| `REMOTR_E2E_CERTS` | `compose/runtime/certs` | Cert directory |
| `REMOTR_E2E_BOOTSTRAP_TOKEN_FILE` | `compose/runtime/bootstrap.token` | Bootstrap token path |
| `REMOTR_E2E_DATABASE_URL` | `postgres://remotr:remotr@127.0.0.1:5432/remotr?sslmode=disable` | Postgres for enroll tests |
| `REMOTR_E2E_ENROLL_TOKEN` | (unset) | Override enrollment token in tests |
| `REMOTR_E2E_RUNTIME` | `compose/runtime` | Runtime state directory |
| `REMOTR_E2E_SERVER_CONTAINER` | `compose-remotr-server-1` | Docker container name for bootstrap fallback |

## Production checklist

Minimum server environment for a real deployment:

```bash
REMOTR_LISTEN=:8443
REMOTR_CONFIG_REPO=/var/lib/remotr/config-repo
REMOTR_DATABASE_URL=postgres://...
REMOTR_TLS_CERT=/etc/remotr/certs/server.crt
REMOTR_TLS_KEY=/etc/remotr/certs/server.key
REMOTR_TLS_CLIENT_CA=/etc/remotr/certs/ca.crt
REMOTR_CA_CERT=/etc/remotr/certs/ca.crt
REMOTR_CA_KEY=/etc/remotr/certs/ca.key
REMOTR_GIT_REMOTE_URL=https://github.com/org/remotr-config.git
REMOTR_GIT_TOKEN=ghp_...
REMOTR_GIT_BRANCH=main
REMOTR_GIT_SYNC_POLL_INTERVAL=10m
REMOTR_GIT_WEBHOOK_SECRET=<random>
```

See `server.env.example` in a scaffolded configuration repository for a copy-paste template.
