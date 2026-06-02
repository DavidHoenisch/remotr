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
- If empty after a previous bootstrap, run `make compose-down && make compose-up` for a fresh database volume.
- `test-e2e-quick` skips admin tests when the stack was already bootstrapped.

### Enrollment returns `401 invalid or expired token`

- Token was already consumed (one-time use).
- Token expired — create a new one with `remotr enroll token create`.
- Fleet not registered in `fleet_settings`.
- Wrong fleet name on token vs. expected enrollment.

### `artifact unavailable` on sync

- `REMOTR_CONFIG_REPO` path wrong or unreadable on server.
- Fleet name on endpoint does not match any `fleets/<name>/desired.yaml`.
- Endpoint override path typo — must be `endpoints/<exact-endpoint-id>/desired.yaml`.

### Git sync not advancing release ref

- Config repo must be a Git checkout (`.git` present) unless using static `REMOTR_RELEASE_REF`.
- Set `REMOTR_GIT_REMOTE_URL` and verify server can `git fetch`.
- Webhook: confirm `X-Remotr-Git-Webhook-Secret` matches `REMOTR_GIT_WEBHOOK_SECRET`.
- Check server logs for `release ref advanced`.

## Agent

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
- Validate YAML with unit tests or a dry review — parser errors fail at apply time.

### Permission denied on `/var/lib/remotr`

Agent runs as root in production. Credential dir is mode `0700` by design.

Compose e2e relaxes bind-mount permissions via `REMOTR_COMPOSE_E2E=1` and Makefile `chmod` — not for production.

## Operator CLI

### `operator credentials missing`

Run `remotr bootstrap` first. Confirm `--state-dir` matches where credentials were saved (default `~/.config/remotr`).

### Admin commands fail with TLS error

- `--ca` must point to Remotr CA PEM (same as agents use).
- `--server-url` must match server certificate SAN/CN or use correct hostname.

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

# Unit tests
make test

# Full integration
make test-e2e
```

## Getting help

1. Reproduce with `make test-e2e` — if it passes, compare your env to [Environment variables](../reference/environment-variables.md).
2. Review [Architecture](../explanation/architecture.md) for identity and artifact path rules.
3. Certificate issues: [CA rotation runbook](../runbooks/ca-rotation.md).
