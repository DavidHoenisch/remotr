# Environment variables reference

## remotr-server

| Variable | Default | Description |
|----------|---------|-------------|
| `REMOTR_LISTEN` | `:8443` | HTTPS listen address |
| `REMOTR_CONFIG_REPO` | `/config-repo` | Absolute path to configuration repository checkout |
| `REMOTR_RELEASE_REF` | `dev` | Static release label when Git sync is inactive |
| `REMOTR_DATABASE_URL` | (unset) | Postgres DSN. Required at server startup for the endpoint/operator registry and durable change-control state. The server exits when it is unset. |
| `REMOTR_TLS_CERT` | `/certs/server.crt` | Server TLS certificate PEM |
| `REMOTR_TLS_KEY` | `/certs/server.key` | Server TLS private key PEM |
| `REMOTR_TLS_CLIENT_CA` | `/certs/ca.crt` | CA for verifying agent and operator client certificates |
| `REMOTR_CA_CERT` | `/certs/ca.crt` | Remotr CA certificate (issues endpoint and operator certs) |
| `REMOTR_CA_KEY` | `/certs/ca.key` | Remotr CA private key |
| `REMOTR_BOOTSTRAP_FILE` | `/var/lib/remotr/bootstrap.token` | One-time operator bootstrap token file |
| `REMOTR_SECRETS_ENABLED` | `false` | Enables Remotr-managed encrypted secrets. Startup fails closed unless exactly one external KEK keyring source is configured. |
| `REMOTR_SECRET_KEK_KEYRING` | (unset) | Path to a root-owned regular JSON keyring file with mode `0600`; symlinks are rejected. Suitable only when the server process can read the root-only file. |
| `REMOTR_SECRET_KEK_KEYRING_B64` | (unset) | Base64-encoded keyring JSON supplied by a deployment secret mechanism. Mutually exclusive with `REMOTR_SECRET_KEK_KEYRING`. |
| `REMOTR_GIT_REMOTE_URL` | (unset) | Git remote URL for sync (HTTPS recommended for PAT) |
| `REMOTR_GIT_TOKEN` | (unset) | GitHub/Git HTTPS personal access token (never stored in git config) |
| `REMOTR_GIT_USERNAME` | `x-access-token` | HTTPS Git username when using `REMOTR_GIT_TOKEN` (GitHub PAT default) |
| `REMOTR_GIT_BRANCH` | `main` | Branch tracked for release ref |
| `REMOTR_GIT_SYNC_POLL_INTERVAL` | `0` (disabled) | Periodic Git sync interval (for example `5m`, `15m`) |
| `REMOTR_GIT_WEBHOOK_SECRET` | (unset) | Validates `X-Remotr-Git-Webhook-Secret` on webhook POST |
| `REMOTR_SYNC_MAX_CONCURRENT` | `0` (no explicit cap) | Maximum concurrent Sync requests admitted by the server. Excess requests receive retry guidance. Size from load evidence. |
| `REMOTR_SYNC_RETRY_AFTER` | `5s` | Retry delay returned by sync admission control. |
| `REMOTR_ARTIFACT_PRUNE_AGE` | `2160h` (90 days) | Compiled artifacts older than this are pruned after successful composition. `0` disables pruning. Invalid values fall back to 90 days. |
| `REMOTR_S3_BUCKET` | (unset) | Private custom-app bucket. Falls back to `BUCKET_NAME`; without either, S3 app upload/download is disabled. |
| `REMOTR_S3_REGION` | `us-east-1` through SDK | S3 region. Falls back to `AWS_REGION`. |
| `REMOTR_S3_ENDPOINT` | AWS default | S3-compatible endpoint. Falls back to `AWS_ENDPOINT_URL_S3`. |
| `REMOTR_S3_PRESIGN_TTL` | `30m` | Lifetime of endpoint custom-package download URLs. |
| `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN` | SDK chain | Server-only S3 credentials. Never place them on endpoints or operator workstations. |

Public CA distribution (no auth): `GET /v1/ca.pem` returns the Remotr CA certificate PEM. The install script only uses this bootstrap endpoint when `REMOTR_CA_FINGERPRINT` pins the expected CA; otherwise provide CA material with `REMOTR_CA_FILE` or `REMOTR_CA_PEM`.

When `REMOTR_SECRETS_ENABLED=true`, configure exactly one KEK source. The file
loader requires a regular, non-symlinked file owned by UID 0 with no
group/other permission bits. The current server passes UID 0 as that ownership
requirement even when its process runs as another user. A non-root service
therefore normally uses strict standard base64 in
`REMOTR_SECRET_KEK_KEYRING_B64`, delivered by the deployment secret manager,
rather than weakening the file mode.

## install-agent.sh

Environment variables for `scripts/install-agent.sh` (see [Installing the agent](../guides/installing-agent.md)):

| Variable | Default | Description |
|----------|---------|-------------|
| `REMOTR_SERVER_URL` | (required) | Server base URL |
| `REMOTR_DEPLOYMENT_TOKEN` | (unset) | Reusable deployment enrollment token |
| `REMOTR_ENROLL_TOKEN` | (unset) | One-time enrollment token |
| `REMOTR_ENROLL_TOKEN_FILE` | (unset) | Path to token file |
| `REMOTR_CA_FINGERPRINT` | (unset) | Required sha256 fingerprint pin for the default `/v1/ca.pem` bootstrap download; optional for out-of-band CA sources |
| `REMOTR_CA_FILE` / `REMOTR_CA_PEM` / `REMOTR_CA_URL` | (unset) | Override default pinned fetch from `/v1/ca.pem` |
| `REMOTR_VERSION` | `latest` | GitHub release version |
| `REMOTR_GITHUB_REPO` | `DavidHoenisch/remotr` | GitHub release owner/repository. |
| `REMOTR_BIN_DIR` | `/usr/local/bin` | Binary installation directory. |
| `REMOTR_STATE_DIR` | `/var/lib/remotr` | Enrolled credential and durable agent state directory. |
| `REMOTR_CONFIG_DIR` | `/etc/remotr` | CA and generated agent environment directory. |
| `REMOTR_SYNC_INTERVAL` | `30s` | Value written to the systemd agent environment. |
| `REMOTR_DEFER_ENROLL` | (unset) | Write `enroll.env` and enroll on first boot via systemd |
| `REMOTR_SKIP_ENROLL` | (unset) | Install binary only; use for upgrades (`REMOTR_VERSION` + restart) |
| `REMOTR_YES` | (unset) | Skip install confirmation prompt |
| `REMOTR_SKIP_SYSTEMD` | (unset) | Install binary and CA without systemd units. |
| `REMOTR_FORCE_ENROLL` | (unset) | Pass `--force` to the agent enrollment command. |
| `REMOTR_VERIFY_CHECKSUMS` | (unset) | Verify the release archive against `checksums.txt`. |
| `REMOTR_ENDPOINT_ID` | hostname-derived | Explicit stable endpoint identifier. |

## remotr-agent

| Variable | Default | Description |
|----------|---------|-------------|
| `REMOTR_SERVER_URL` | `https://remotr-server:8443` | Server base URL |
| `REMOTR_SYNC_INTERVAL` | `30s` | Sync poll interval |
| `REMOTR_SYNC_TIMEOUT` | `120s` | Per-request timeout for `POST /v1/sync` |
| `REMOTR_SYSTEM_INFO_INTERVAL` | `1h` | Normal interval for sending changed system-inventory snapshots. A five-minute minimum is enforced by the throttler. |
| `REMOTR_BIN_DIR` | `/usr/local/bin` | Target directory for in-band agent self-upgrade |
| `REMOTR_STATE_DIR` | `/var/lib/remotr` | Enrolled credential directory (`agent.crt`, `agent.key`, `ca.crt`, `state.json`) |
| `REMOTR_ENROLL_TOKEN` | (unset) | One-time enrollment token (enroll subcommand) |
| `REMOTR_ENROLL_TOKEN_FILE` | (unset) | Absolute path to enrollment token file |
| `REMOTR_ENDPOINT_ID` | (unset) | Stable endpoint identifier (hostname-based default) |
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
| `NO_COLOR` | (unset) | Disable color labels even when `--color auto` would enable them. |
| `REMOTR_DEMO` | (unset) | When `1` or `true`, admin API calls use static fixtures instead of the network (for docs and VHS recordings) |
| `REMOTR_DEMO_FIXTURES` | (unset) | Directory of HTTP fixture JSON files (required when `REMOTR_DEMO` is set). Default in Makefile: `demo/fixtures/http` |

CLI flags override config file values; config file overrides built-in defaults.

Global flags (`--server-url`, `--state-dir`, `--ca`, `--fleet`, `--verbose`) may appear before or after the subcommand.

### Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Runtime or API error, including a failed cron execution report. |
| `2` | Usage, configuration, or missing credentials |
| `4` | Compliance drift (state report commands) |

Use `--verbose` for additional error detail. Structured errors include a `code:` field (for example `E_CREDENTIALS_MISSING`). Shell integrations can query candidates with the framework flag `remotr --generate-shell-completion`; the repository does not currently ship a completion-script installer. Run `remotr doctor` to diagnose setup.

### Demo mode and VHS recordings

Set `REMOTR_DEMO=1` and `REMOTR_DEMO_FIXTURES` to the fixture directory so `remotr` commands behave normally but read admin API responses from JSON files under `demo/fixtures/http` (see `make demo-fixtures`). The Makefile targets `demo-record` and `demo-record-all` set these variables when invoking [VHS](https://github.com/charmbracelet/vhs); do not put them in `.tape` files or they appear in generated GIFs.

```bash
make demo-fixtures          # regenerate demo TLS + bootstrap fixture
make demo-record TAPE=init  # record one GIF to demo/assets/
make demo-record-all        # record all operator-workflow GIFs
```

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
REMOTR_SECRETS_ENABLED=true
# Root-running server with a root-owned 0600 file:
REMOTR_SECRET_KEK_KEYRING=/etc/remotr/secrets/kek-keyring.json
REMOTR_GIT_REMOTE_URL=https://github.com/org/remotr-config.git
REMOTR_GIT_TOKEN=ghp_...
REMOTR_GIT_BRANCH=main
REMOTR_GIT_SYNC_POLL_INTERVAL=10m
REMOTR_GIT_WEBHOOK_SECRET=<random>
```

For a non-root service, omit the file variable and have the deployment secret
manager set `REMOTR_SECRET_KEK_KEYRING_B64` instead. Never set both.

The external keyring document has one active encryption key and may retain
historical decrypt-only keys. Values are standard base64 encodings of exactly
32 random bytes:

```json
{
  "active": "kek-2026-07",
  "keys": {
    "kek-2026-07": "<base64-encoded-32-byte-key>",
    "kek-2026-06": "<base64-encoded-32-byte-historical-key>"
  }
}
```

Back up this keyring independently of Postgres. Encrypted secret records are
not recoverable from a database backup alone. On restore, install the matching
keyring with root ownership and mode `0600` before starting the server; Remotr
does not generate a replacement KEK when configured material is absent or
invalid. With secrets enabled, startup validates every restored encrypted
record and refuses to serve when its referenced KEK is missing.

See `server.env.example` in a scaffolded configuration repository for a copy-paste template.
