# Remotr implementation checklist

Track progress against the design in `CONTEXT.md`. Run `make test` (unit) and `make test-e2e` (Docker stack).

## Done

- [x] Domain glossary (`CONTEXT.md`)
- [x] YAML `ParseState` + `Configuration` slices in one artifact
- [x] `cmd/remotr-server`, `cmd/remotr-agent`, `cmd/remotr` (operator CLI)
- [x] chi server: `GET /healthz`, `POST /v1/sync`, `POST /v1/enroll`, admin API
- [x] mTLS sync; endpoint identity from cert SAN (no self-reported ID)
- [x] GitOps config repo path: `fleets/<fleet>/desired.yaml`
- [x] Endpoint override path: `endpoints/<id>/desired.yaml`
- [x] Docker Compose: Postgres, cert init, Debian + Arch agents, sample config
- [x] E2E: healthz, sync, enroll, admin bootstrap, gzip sync
- [x] `vendor/` builds; allowlist: chi, yaml.v3 (+ pgx, uuid for Postgres)
- [x] Postgres schema (`sql/schema.sql`) + sqlc queries
- [x] `internal/store/postgres` registry store + telemetry tables
- [x] `POST /v1/enroll` + Remotr CA issuance (server-key legacy + CSR default)
- [x] Agent enroll subcommand (CSR, credential storage, sync loop)
- [x] Admin CLI: `init`, `bootstrap`, `enroll token create`, `endpoint list/show`
- [x] Operator credentials + mTLS admin API
- [x] Git sync webhook + poll (`Release ref` advancement)
- [x] Agent: parse artifact → resolved desired state (in-document targeting)
- [x] Check / Apply: packages (apt, dnf, pacman), files, users
- [x] Systemd + Command resources; `depends_on`, apply order, pre-apply validation
- [x] Drift + apply failure telemetry to Postgres
- [x] Per-fleet remediation policy on sync response
- [x] Sync response gzip (server + client)
- [x] CSR-based enroll (agent-generated keys)
- [x] Labels in admin API / CLI queries
- [x] Compose stack: operator bootstrap, enrollment tokens, agent CSR enroll, mTLS sync (no pre-baked endpoint certs)
- [x] Production CA / cert rotation runbook (`docs/runbooks/ca-rotation.md`)
- [x] Fly.io + Neon bootstrap installer (`deploy/fly/bootstrap.sh`)
- [x] User documentation (`docs/` — tutorial, guides, reference, architecture)
- [x] ADR: supply chain allowlist (`docs/adr/001-vendored-allowlist.md`)
- [x] ADR: Postgres vs SQLite (`docs/adr/002-postgres-server-registry.md`)

## Testing

- [x] Go fuzz targets (`make fuzz-short` / `make fuzz FUZZ_TIME=5m`)
- [x] Fleet path traversal hardening in `configrepo`
- [x] `make gosec` (G204/G304/G112 remediated; sqlc output excluded via `-exclude-generated`)
