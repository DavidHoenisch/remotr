# Architecture

Remotr separates **desired state** (Git), **operational registry** (Postgres), and **execution** (agents). The server is a control plane that serves artifacts and issues credentials; it does not SSH to machines or merge configuration layers at runtime.

## System context

```text
┌─────────────────────┐         ┌──────────────────────┐
│ Configuration repo  │  fetch  │    remotr-server     │
│ (Git)               │ ──────► │  - release ref       │
│ fleets/*/desired    │ webhook │  - artifact + digest │
│ endpoints/*/desired │  poll   │  - enroll + admin    │
└─────────────────────┘         └──────────┬───────────┘
                                           │ mTLS HTTPS
                              ┌────────────┴────────────┐
                              ▼                         ▼
                    ┌─────────────────┐       ┌─────────────────┐
                    │  remotr-agent   │       │  remotr (CLI)   │
                    │  - resolve      │       │  operator cred  │
                    │  - check/apply  │       └─────────────────┘
                    └─────────────────┘
```

## Three binaries

| Binary | Runs on | Responsibility |
|--------|---------|----------------|
| `remotr-server` | Central infrastructure | TLS termination, registry, artifact serving, CA issuance, Git sync |
| `remotr-agent` | Each Linux endpoint | Enroll, sync loop, resolve targeting, check, apply |
| `remotr` | Operator workstation | Bootstrap, enrollment tokens, endpoint inventory; GitOps scaffolding |

Shared libraries live under `internal/`. Production builds use vendored dependencies only.

## Trust and identity

### Remotr CA

The server holds the CA key (`REMOTR_CA_KEY`). It signs:

- Server TLS certificate
- Endpoint client certificates (at enroll)
- Operator client certificates (at bootstrap)

Agents trust the CA via `REMOTR_TLS_CA` / stored `ca.crt`.

### Authenticated endpoint identity

On every `/v1/sync` request:

1. TLS handshake presents the endpoint client certificate.
2. Server maps certificate fingerprint (and SAN) to exactly one row in `endpoints`.
3. Fleet assignment and artifact path come from that row.

A compromised agent cannot impersonate another endpoint by sending a different ID in JSON — the body carries telemetry only.

### Operator vs endpoint credentials

Same CA, different ACL:

- Endpoint certs → `/v1/sync` only
- Operator certs → `/v1/admin/*`

Bootstrap uses a one-time token instead of mTLS for the first operator.

## From artifact to apply

```text
Deployable artifact (YAML)
        │
        ▼
   Parse configurations[]
        │
        ▼
 In-document targeting (agent)
 targetDistros / targetArch
        │
        ▼
 Resolved desired state
        │
        ├─► Check  ──► drift report ──► server telemetry
        │
        └─► Apply  ──► applicators (packages, files, users, systemd, commands)
              │
              └──► revert on resource failure
```

### Artifact resolution on the server

For endpoint `E` in fleet `F`:

1. If `endpoints/<E>/desired.yaml` exists in the config repo → serve it.
2. Else serve `fleets/<F>/desired.yaml`.

No merge. Git is responsible for composition before push.

### Release ref

One global release ref (commit SHA) for v1. When Git sync advances it, all endpoints may receive new artifact bytes on next sync if the digest changed.

Agents send `lastDigest` to skip redundant downloads.

### Remediation policy

Stored per fleet in Postgres. Returned on every sync response:

- **auto** — apply when check finds drift
- **report** — record drift, skip apply

Policy is server-authoritative; agents do not infer it from YAML.

## Apply engine

Resources are ordered by:

1. Explicit `dependsOn` graph (must be acyclic)
2. Default class order: packages → files → users → systemd → commands
3. Critical `/etc` files after non-critical files

Each resource is atomic: failure triggers revert for that resource only.

`preApplyValidation` runs before mutating sensitive resources (for example `sshd -t`).

## Server registry (Postgres)

Not in Git:

| Data | Purpose |
|------|---------|
| `endpoints` | ID, fleet, cert fingerprint |
| `enrollment_tokens` | One-time enroll secrets |
| `operator_credentials` | Operator cert fingerprints |
| `fleet_settings` | Remediation policy |
| `release_ref` | Current Git SHA |
| Drift / apply telemetry | Last reports from sync body |

In-memory registry exists for unit tests; production requires Postgres.

## Git sync

`internal/gitsync` resolves `HEAD` after optional `git fetch`, compares to stored ref, persists on change. Webhook and poll share the same `Sync()` path.

Non-Git config mounts use static `REMOTR_RELEASE_REF` — suitable for dev Compose (`e2e-dev`).

## Security properties

| Property | Mechanism |
|----------|-----------|
| No inbound agent ports | Pull-only sync |
| Mutual TLS | Client certs for sync and admin |
| Least privilege on admin | Separate operator credentials |
| Supply chain | Vendored allowlist (see ADR 001) |
| Path traversal hardening | Config repo path validation |

## What the server does not do

- Merge global + fleet + label + endpoint YAML at runtime
- Execute commands on endpoints
- Provide a web admin UI (v1)
- Integrate enterprise PKI (v1; Remotr CA only)

## Further reading

- Domain glossary: [CONTEXT.md](../../CONTEXT.md)
- [Configuration repository guide](../guides/configuration-repository.md)
- [HTTP API](../reference/http-api.md)
- [ADR: Postgres registry](../adr/002-postgres-server-registry.md)
