## Why

Steady-state agents repeatedly send the same Sync inputs, but the server still resolves artifacts and rewrites liveness, capability, delivery, inventory, and audit state through Postgres on every poll. At fleet scale, removing that avoidable database work provides a larger near-term benefit than changing Remotr's pull cadence and preserves prompt delivery when authoritative state changes. Process-local caching alone is insufficient for Fly Machines that stop when idle or for multiple serving Machines, so deployments also need an optional shared Redis backend.

## What Changes

- Add selectable `disabled`, bounded in-memory, and bounded Redis-backed decision-cache modes that can answer an authenticated, side-effect-free unchanged Sync without reading or writing Postgres.
- Invalidate affected cache entries before revocation, remediation-policy, release, enrollment, targeting, or other response-affecting mutations become observable; Redis mode coordinates that barrier synchronously across serving processes.
- Extend the agent Sync protocol with stable hashes for capability, delivery acknowledgement, system information, and other repeatable documents; send full documents only when their hash changes or the server explicitly requests them.
- Persist capability documents, delivery state, and system information only when their semantic content changes.
- Coalesce steady-state `last_seen` and successful unchanged-Sync audit persistence into periodic durability checkpoints, defaulting to five minutes and bounded to at most ten minutes, while preserving immediate security- and mutation-related audit records.
- Add cache correctness, invalidation, concurrency, restart, authenticated load, database-operation, allocation, and payload evidence for the new hot path.
- Update operator, environment, deployment, and Fly bootstrap documentation. The Fly bootstrap creates or reuses a private Upstash Redis database through `fly redis create`, imports its private URL as a Remotr secret, and selects the Redis backend unless explicitly skipped.
- Defer adaptive success-poll backoff; polling cadence, bounded jitter, transient retry, and Remotr's pull architecture remain unchanged.

## Capabilities

### New Capabilities

- `unchanged-sync-fast-path`: Defines eligibility, hash-based document elision, selectable memory or Redis-backed unchanged decisions, synchronous invalidation, changed-only persistence, and bounded liveness/audit checkpointing.

### Modified Capabilities

- `performance-and-scale-assurance`: Extends the authenticated unchanged-Sync workload and budgets to measure cache effectiveness and prove the steady-state fast path performs no database operations between checkpoints.

## Impact

The authenticated Sync request/response schema gains backward-compatible hash and document-request fields. The server Sync orchestration, mutation handlers, Git release advancement, enrollment/revocation paths, registry persistence adapters, audit recording, clock boundaries, configuration, Fly bootstrap, and metrics are affected. Agent pending-state and credential-state persistence must retain acknowledged document hashes across restarts. Memory mode requires no external service; Redis mode adds an optional Redis-compatible dependency and permits Fly Machine restart reuse and coordinated multi-process serving. Legacy agents continue through the existing full Sync path.
