# Architecture

Remotr separates **desired state** (Git), **operational registry** (Postgres), and **execution** (agents). The server is a control plane that serves artifacts and issues credentials; it does not SSH to machines or merge configuration layers at runtime.

## System context

```text
┌─────────────────────┐         ┌──────────────────────┐
│ Configuration repo  │  fetch  │    remotr-server     │
│ (Git)               │ ──────► │  - release ref       │
│ kind-tagged YAML    │ webhook │  - compose + cache   │
│ fleets/*/manifest   │  poll   │  - artifact + digest │
│ endpoints/*/manifest│         │  - enroll + admin    │
└─────────────────────┘         └──────────┬───────────┘
                                           │ mTLS HTTPS
                         ┌─────────────────┼─────────────────┐
                         ▼                 ▼                 ▼
               ┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐
               │  remotr-agent   │ │  remotr (CLI)   │ │ remotr-desktop  │
               │  - resolve      │ │  operator cred  │ │ native Linux UI │
               │  - check/apply  │ └─────────────────┘ │ operator cred   │
               └─────────────────┘                     └─────────────────┘
```

The CLI and desktop boxes are peer operator surfaces; neither invokes or embeds
the other. Both use the existing mTLS-protected Admin API directly.

## Binaries and release boundaries

| Binary | Runs on | Responsibility |
|--------|---------|----------------|
| `remotr-server` | Central infrastructure | TLS termination, registry, artifact serving, CA issuance, Git sync |
| `remotr-agent` | Each Linux endpoint | Enroll, sync loop, resolve targeting, check, apply |
| `remotr` | Operator workstation | Bootstrap, enrollment tokens, endpoint inventory; GitOps scaffolding |
| `remotr-desktop` | Linux operator workstation | Native visual Fleet workspace over purpose-specific typed bindings and the existing Admin API |

`remotr-desktop` is an additive Linux release artifact in an isolated
`desktop/` nested module. It does not change or embed `remotr`, and its Wails,
frontend, WebView, and packaging dependencies do not enter the root Go module's
routine build or test graph. Shared root-module libraries live under
`internal/`. Production root-module builds use vendored dependencies only.

The desktop artifact is supported or advertised only after its exact Linux
architecture and package format pass native build, launch, install, and removal
evidence. Until that release gate passes, the Admin CLI remains the documented
operator surface and recovery path.

### Desktop trust boundary

The native Go application service is the trust boundary between the embedded
frontend and the authenticated Admin API. The frontend receives only
purpose-specific typed view models and actions; it does not receive an arbitrary
HTTP client, raw Admin client, filesystem or process primitive, private key,
bootstrap token result, or unrestricted diagnostic bytes. Operator credentials
remain in the existing protected layout and are loaded by the Go backend.

Release builds embed their frontend assets and block in-window remote content.
Remotr Desktop is not hosted by `remotr-server`, does not add a browser-reachable
control-plane service, and does not shell out to the Admin CLI. Git remains the
only desired-state deployment boundary.

## Trust and identity

### Remotr CA

The server holds the CA key (`REMOTR_CA_KEY`). It signs:

- Server TLS certificate
- Endpoint client certificates (at enroll)
- Operator client certificates (at bootstrap)

Agents trust the CA via `REMOTR_TLS_CA` / stored `ca.crt`. The server exposes the public CA at **`GET /v1/ca.pem`** (no authentication) so install scripts and operators can bootstrap trust before enrollment.

### Authenticated endpoint identity

On every `/v1/sync` request:

1. TLS handshake presents the endpoint client certificate.
2. Server maps certificate fingerprint (and SAN) to exactly one row in `endpoints`.
3. Fleet assignment and artifact path come from that row.

A compromised agent cannot impersonate another endpoint by sending a different ID in JSON — the body carries telemetry only.

### Unchanged Sync fast path

After a full authenticated Sync, the server may retain a bounded memory- or Redis-backed
decision for that certificate, endpoint, Fleet authority generation, artifact
delivery identity, and set of server-accepted document hashes. A later request
is eligible only when it contains the same accepted hashes and delivery
acknowledgement and contains no telemetry, report, request, or one-shot work.
An eligible hit returns the already-authorized unchanged response without a
Postgres read or write. Missing, changed, malformed, or unaccepted hashes fall
back to the normal authoritative path; legacy agents continue sending full
documents.

The cache has deterministic entry, byte, observation, TTL, and checkpoint
bounds. Every five minutes by default (configurable from five to ten minutes),
the next otherwise-quiet Sync leaves the fast path and durably records one
check-in plus an aggregate audit event for the completed window. A checkpoint
failure returns an error and remains pending; it is never acknowledged as a
successful quiet hit. Memory starts cold after process replacement. Redis keeps
the bounded decision and checkpoint observation available to another process.

Authority-changing operations invalidate before they mutate. Global changes
include Git release advance and change-control, secret, token, and operator
authority mutations. Fleet changes include remediation policy. Endpoint
changes include enrollment, deletion, Fleet reassignment, labels, upgrade
requests, and diagnostics. Generation barriers prevent a concurrent stale
fill from repopulating invalidated state.

Memory mode is safe only with one serving process. Redis mode uses atomic Lua
operations on the primary for lookup/fill generation checks, checkpoint claims,
and mutation begin/complete barriers; Pub/Sub and read replicas are not
correctness boundaries. Sync falls back to Postgres when Redis is unavailable,
while an authority mutation returns unavailable before durable work if it
cannot establish the shared barrier. Set
`REMOTR_UNCHANGED_SYNC_FAST_PATH=false` for immediate rollback to the
authoritative legacy path.

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
        └─► Apply  ──► typed providers from the resource registry
              │
              └──► bounded rollback according to each resource contract
```

The registry currently covers 48 canonical resource kinds. The complete
vocabulary and each kind's configuration reference are indexed in
[Resource kinds](../reference/resource-kinds.md); support claims are narrower
than schema admission and depend on an exact passing provider row.

### Artifact resolution on the server

For endpoint `E` in fleet `F` at release ref `R`:

1. Load the bounded composed **desired** artifact variants from Postgres cache
   for `(endpoint E, R)`; if missing, use `(fleet F, R)`.
2. Match a variant against the endpoint's current authenticated capability
   document. Never remove a resource or field to manufacture compatibility.
3. Record the target, offered, and eventually acknowledged active artifact as
   separate delivery states.
4. Use the same endpoint-then-fleet resolution pattern for **crons** when
   present.

Composition runs when Git sync advances the release ref: the server discovers `kind: manifest` entry points, merges modules/applications/crons, and upserts `compiled_artifacts`. If composition fails, the release ref does not advance. Without Postgres, the server composes on demand at sync time.

Endpoint override manifests **replace** fleet artifacts (no runtime merge).

### Release ref

One global release ref (commit SHA) for v1. When Git sync advances it, each
endpoint may receive a compatible artifact variant on its next sync. Agents
send `lastReleaseRef`, `lastDigest`, and a current capability document. An
offer becomes active only after a later sync acknowledges the exact release
and digest.

If no variant satisfies the current capability evidence, Sync succeeds with a
bounded `capabilityBlocked` result and no artifact bytes. Existing endpoints
retain their last active artifact; new endpoints remain unmanaged. Agent
version is metadata, not proof of provider support. See
[Capability-compatible delivery](../reference/capability-compatible-delivery.md).

### Remediation policy

Stored per fleet in Postgres. Returned on every sync response:

- **auto** — apply when check finds drift
- **report** — record drift, skip apply

Policy is server-authoritative; agents do not infer it from YAML.

## Server-managed crons

**Desired state** (composed artifact) converges when drift is detected. **Crons** (composed crons artifact) run on a schedule regardless of drift.

```text
Composed crons artifact
        │
        ▼
 Server resolves use: + evaluates schedule + last run (Postgres)
        │
        ▼
 POST /v1/sync ──► dueCrons[] ──► Agent apply-only
        │
        ▼
 Next sync ──► cronResults[] ──► audit in Postgres
```

Crons use the same resource stanzas as desired state but are a separate artifact. The server never writes crontab entries on endpoints. Missed runs while an endpoint is offline are executed once on the next check-in.

Cron artifact resolution mirrors desired state: endpoint override replaces fleet file (no merge).

## Apply engine

The engine builds one global acyclic graph from stable resource addresses and
explicit `dependsOn` edges. Deterministic registry order breaks ties between
otherwise independent resources; authored configuration slices do not create
execution phases. Every provider returns a typed Check outcome, and only
supported drift is eligible for Apply. `preApplyValidation` and provider
preflight checks run before mutation where the contract requires them.

Resources declare ownership, risk, activation, lock domains, and rollback
capability. Locks are acquired in deterministic order. Mutations keep bounded,
protected rollback state and report verification or recovery outcomes through
classified summaries; raw secret values are not a representable report shape.
See [Applicator execution contract](../reference/applicator-execution.md).

## Server registry (Postgres)

Not in Git:

| Data | Purpose |
|------|---------|
| `endpoints` | ID, fleet, cert fingerprint |
| `enrollment_tokens` | One-time enroll secrets |
| `operator_credentials` | Operator cert fingerprints |
| `fleet_settings` | Remediation policy |
| `release_ref` | Current Git SHA |
| `compiled_artifacts` | Bounded composed artifact variants and their requirement sets |
| Endpoint capability documents | Latest valid canonical document, digest, authenticated endpoint identity, and receive time |
| Endpoint delivery state | Target, offered, active, capability-blocked, missing-requirement, and unmanaged state |
| `change_control_state` | Versioned full registry snapshot: requests, approvals, lifecycle and audit history, rollouts, leases, attempts, outcomes and progress, policies, break-glass records, and baselines |
| Structured state reports | Classified Check, Apply, activation, reboot, rollback, and schedule-runtime evidence |
| Protected secrets | Encrypted versions, active-version selection, authorization metadata, and audit records |
| `cron_last_run` / `cron_executions` | Scheduled job dispatch and audit history |

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
| Capability-safe delivery | Authenticated bounded documents, exact requirement matching, and fail-closed selection |
| Secret-safe reporting | Schema-owned sensitivity and projections; raw secret values cannot enter structured summaries |
| Resource recovery | Bounded protected rollback state, ownership boundaries, exact locks, and explicit recovery classes |

## What the server does not do

- Merge global + fleet + label + endpoint YAML at runtime
- Execute commands on endpoints
- Host Remotr Desktop or provide a browser admin UI
- Integrate enterprise PKI (v1; Remotr CA only)

## Further reading

- Domain glossary: [Terminology](terminology.md)
- [Configuration repository guide](../guides/configuration-repository.md)
- [Applicator execution contract](../reference/applicator-execution.md)
- [Capability-compatible delivery](../reference/capability-compatible-delivery.md)
- [HTTP API](../reference/http-api.md)
- [ADR: Postgres registry](https://github.com/DavidHoenisch/remotr/blob/master/engineering/adr/002-postgres-server-registry.md)
