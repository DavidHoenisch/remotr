# Troubleshooting

Common issues when running Remotr locally (Compose) or in production.

## Server

### `GET /healthz` fails or connection refused

- Confirm `remotr-server` is listening on `REMOTR_LISTEN` (default `:8443`).
- Check TLS: use `curl -k` only for dev with self-signed certs; production clients need the Remotr CA.
- Docker Compose: `docker compose -f compose/docker-compose.yml ps` — server should be `healthy`.

### Bootstrap token missing or empty

The server emits a bootstrap token only when Postgres has **no registered operators**.

- Read `compose/runtime/bootstrap.token` or `docker logs compose-remotr-server-1`.
- The Compose token file is root-owned mode `0600`; use `sudo cat compose/runtime/bootstrap.token`.
- If empty after a previous bootstrap, run `make compose-down && make compose-up` for a fresh database volume.
- `test-e2e-quick` skips admin tests when the stack was already bootstrapped.

### Enrollment returns `401 invalid or expired token`

- Token was already consumed (one-time use).
- Token expired — create a new one with `remotr enroll token create`.
- Fleet not registered in `fleet_settings`.
- Wrong fleet name on token vs. expected enrollment.

### `artifact unavailable` on sync

- `REMOTR_CONFIG_REPO` path wrong or unreadable on server.
- Fleet name on endpoint does not match a fleet with a valid `kind: manifest` under `fleets/<name>/`.
- Endpoint override typo — path must be `endpoints/<exact-endpoint-id>/manifest.yaml`.
- Composition failed on last git sync — check server logs for `compose at` or `initial git sync` errors; release ref may be stuck on the previous commit.
- Empty `compiled_artifacts` after upgrading to v0.4.x — run `remotr git sync` (or restart the server on v0.4.2+) to populate the cache; ensure `REMOTR_GIT_REMOTE_URL` points at the migrated config repo and `REMOTR_GIT_BRANCH` matches your default branch (for example `master`).
- Run `remotr config validate .` on the config repo to catch reference and merge errors before push.

### Git sync not advancing release ref

- Config repo must be a Git checkout (`.git` present) unless using static `REMOTR_RELEASE_REF`.
- Set `REMOTR_GIT_REMOTE_URL` and `REMOTR_GIT_TOKEN` (private GitHub) and verify server logs show `release ref advanced`.
- Webhook: confirm `X-Remotr-Git-Webhook-Secret` matches `REMOTR_GIT_WEBHOOK_SECRET`.
- Check server logs for `release ref advanced`.

### Server fails while loading the KEK keyring

- Configure exactly one of `REMOTR_SECRET_KEK_KEYRING` and
  `REMOTR_SECRET_KEK_KEYRING_B64`.
- The file source must be a non-symlinked regular file owned by UID 0 with no
  group/other permission bits.
- The current file loader enforces UID 0 even for a non-root server process. A
  dedicated service account should receive the complete keyring through the
  base64 deployment-secret variable instead of relaxing file permissions.
- Restore every historical KEK referenced by Postgres; generating a new active
  key does not recover old ciphertext.

### Change-control state does not load after restart

- Confirm `REMOTR_DATABASE_URL` points to the restored production database;
  the server now refuses to start without the durable Postgres registry.
- Apply `sql/migrations/016_change_control_state.sql` (normally through
  `make migrate`) before starting the new binary.
- Inspect server logs for a state version, validation, or revision-conflict
  error. Do not delete the row or recreate approvals to make startup succeed.
- Confirm the database backup includes the singleton
  `change_control_state` row and was restored consistently with encrypted
  secret activation records.
- After recovery, compare `remotr change show <id> --json` with the external
  change record. Request IDs, approvals, issued leases, and attempt accounting
  survive when their mutations reached Postgres.
- Verify that restored `pause` or `revoke` state and its audit entry match the
  external change record. Do not recreate approvals to repair a mismatch.

See [Change-control restart recovery](change-control.md#server-restart-recovery).

## Agent

### Agent install script

See [Installing the agent](installing-agent.md) for the full flow.

| Symptom | Fix |
|---------|-----|
| `/dev/fd/N: No such file or directory` / `curl: (23) Failure writing output` | Do not use `sudo bash <(curl ...)`. Use `curl ... \| sudo REMOTR_*=... bash` or run from a root shell (`sudo -i`) |
| `run as root` | Script ran as your normal user. Use `curl ... \| sudo REMOTR_*=... bash`, or stay in the root shell after `sudo -i` until install finishes (do not `exit` first) |
| `no terminal` / `aborted` | Set `REMOTR_YES=1` on `sudo bash`, or run `bash <(curl ...)` from a root shell |
| `failed to download CA` | Check `REMOTR_SERVER_URL`, firewall, and that the server runs with a CA configured; for the default server CA download, provide `REMOTR_CA_FINGERPRINT` from a trusted admin channel |
| `CA fingerprint mismatch` | Verify the pin against a trusted CA copy or server console and update `REMOTR_CA_FINGERPRINT` |
| `enrollment token required` | Set `REMOTR_DEPLOYMENT_TOKEN` or `REMOTR_ENROLL_TOKEN` |
| `could not resolve latest release` | Pin `REMOTR_VERSION=v1.x.x` or install `jq`; ensure a GitHub release exists |
| `unsupported architecture` | Linux amd64/arm64 only |

### `enroll: credentials already exist`

Use `--force` to replace credentials (re-enrollment):

```bash
remotr-agent enroll --force --token ... 
```

### `enroll: token required`

Provide token via `--token`, `REMOTR_ENROLL_TOKEN`, or `REMOTR_ENROLL_TOKEN_FILE` (absolute path).

### Sync fails with TLS / certificate errors

- Verify `REMOTR_TLS_CA` matches the CA that signed the server cert.
- Confirm enrolled credentials in `REMOTR_STATE_DIR` are complete (`agent.crt`, `agent.key`, `ca.crt`, `state.json`).
- After CA rotation, re-enroll all agents with new CA trust bundle.

### Agent enrolled but desired state not applied

- Check fleet **remediation policy** — `report` records drift without apply.
- Inspect agent logs for `pipeline failed` or applicator errors.
- Confirm configuration slice matches agent distro/arch (`targetDistros`, `targetArch`).
- Run `remotr config validate` and inspect the server's active release ref.
  Composition errors block release advance; an older valid artifact may still
  be active.

### Permission denied on `/var/lib/remotr`

Agent runs as root in production. Credential dir is mode `0700` by design.

Compose e2e relaxes bind-mount permissions via `REMOTR_COMPOSE_E2E=1` and Makefile `chmod` — not for production.

### In-band agent upgrade fails

| Symptom | Fix |
|---------|-----|
| `text file busy` on `/usr/local/bin/remotr-agent` | Upgrade agent to **v0.1.15+**, or stop the service and use [manual install](agent-deployment.md#manual-install-script) |
| `download … 404` | Confirm the GitHub release tag exists and publishes `remotr-agent_*_linux_*` assets |
| Upgrade requested every sync | Check `remotr endpoint show` — taint clears when reported version matches desired with phase `completed` |
| No `agentUpgrade` in sync | Server migration `003_agent_upgrade.sql` not applied, or versions already match |

```bash
journalctl -u remotr-agent -f
remotr endpoint show <endpoint-id> --json
```

## Operator CLI

### `operator credentials missing`

Run `remotr bootstrap` first. Confirm `--state-dir` matches where credentials were saved (default `~/.config/remotr`).

### Admin commands fail with TLS error

- `--ca` must point to Remotr CA PEM (same as agents use).
- `--server-url` must match server certificate SAN/CN or use correct hostname.

### `flag provided but not defined`

Use global operator flags (`--server-url`, `--config`, `--state-dir`) documented
in the [CLI reference](../reference/cli.md), or persist them with `remotr config
init`. Current builds accept globals before or after subcommands. If an older
binary rejects the placement, run `remotr version`, put globals before the
subcommand, and upgrade deliberately.

### `remotr init` says the directory is not empty

This is deliberate overwrite protection. Choose a new empty directory. To
register a fleet and create an enrollment token during scaffolding, include
`--register-server --enroll` on the first `init` call; do not scaffold and then
rerun `init` against the same directory.

### Canonical module rejects `present` or plural collections

New `kind: module` files use `schemaVersion: 1`, one `resources:` list, and
`lifecycle` where the resource kind supports it. Compact `kind: application`
files and cron job bodies intentionally retain their kind-specific package or
plural collection shapes. See [Repository file kinds](../reference/repository-kinds.md).

### `endpoint list` empty but agents running

- Agents may not have enrolled yet — wait for health check or inspect `/var/lib/remotr/state.json` in container.
- Different Postgres instance than server (check `REMOTR_DATABASE_URL`).

## Docker Compose dev stack

### E2E test: `agent did not finish enrollment`

- Agents need server healthy + valid enroll token in `compose/runtime/enroll-tokens/`.
- Run `make compose-down` to clear stale agent state, then `make test-e2e`.
- If host cannot read `compose/runtime/agent-debian/`, run the Makefile target (fixes container dir permissions).

### Stale containers after refactor

```bash
make compose-down
docker compose -f compose/docker-compose.yml up -d --build --wait --remove-orphans
```

## Diagnostics commands

```bash
# Server health
curl -k https://localhost:8443/healthz

# Server logs
docker logs compose-remotr-server-1

# Agent credentials inside container
docker exec compose-agent-debian-1 ls -la /var/lib/remotr/

# Agent sync logs
docker logs compose-agent-debian-1 --tail 50

# List endpoints (after bootstrap)
go run -mod=vendor ./cmd/remotr endpoint list \
  --server-url https://localhost:8443 \
  --state-dir ./compose/runtime/operator

# Collect diagnostics from an endpoint (requires S3/MinIO and diagnostics_collector role)
go run -mod=vendor ./cmd/remotr diagnostics collect ENDPOINT_ID \
  --server-url https://localhost:8443 \
  --state-dir ./compose/runtime/operator

# Save bundle to disk instead of interactive viewer
go run -mod=vendor ./cmd/remotr diagnostics collect ENDPOINT_ID --save /tmp/diag.tar.gz

# Unit tests
make test

# Full integration
make test-e2e
```

## Getting help

1. Reproduce with `make test-e2e` — if it passes, compare your env to [Environment variables](../reference/environment-variables.md).
2. Review [Architecture](../explanation/architecture.md) for identity and artifact path rules.
3. Certificate issues: [CA rotation runbook](../runbooks/ca-rotation.md).
