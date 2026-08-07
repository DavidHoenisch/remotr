## Why

Steady-state agents repeatedly send the same Sync inputs, but the server still resolves artifacts and rewrites liveness, capability, delivery, inventory, and audit state through Postgres on every poll. At fleet scale, removing that avoidable database work provides a larger near-term benefit than changing Remotr's pull cadence and preserves prompt delivery when authoritative state changes.

## What Changes

- Add a bounded in-memory decision cache that can answer an authenticated, side-effect-free unchanged Sync without reading or writing the database.
- Invalidate affected cache entries before revocation, remediation-policy, release, enrollment, targeting, or other response-affecting mutations become observable.
- Extend the agent Sync protocol with stable hashes for capability, delivery acknowledgement, system information, and other repeatable documents; send full documents only when their hash changes or the server explicitly requests them.
- Persist capability documents, delivery state, and system information only when their semantic content changes.
- Coalesce steady-state `last_seen` and successful unchanged-Sync audit persistence into periodic durability checkpoints, defaulting to five minutes and bounded to at most ten minutes, while preserving immediate security- and mutation-related audit records.
- Add cache correctness, invalidation, concurrency, restart, authenticated load, database-operation, allocation, and payload evidence for the new hot path.
- Defer adaptive success-poll backoff; polling cadence, bounded jitter, transient retry, and Remotr's pull architecture remain unchanged.

## Capabilities

### New Capabilities

- `unchanged-sync-fast-path`: Defines eligibility, hash-based document elision, in-memory unchanged decisions, immediate invalidation, changed-only persistence, and bounded liveness/audit checkpointing.

### Modified Capabilities

- `performance-and-scale-assurance`: Extends the authenticated unchanged-Sync workload and budgets to measure cache effectiveness and prove the steady-state fast path performs no database operations between checkpoints.

## Impact

The authenticated Sync request/response schema gains backward-compatible hash and document-request fields. The server Sync orchestration, mutation handlers, Git release advancement, enrollment/revocation paths, registry persistence adapters, audit recording, clock boundaries, and metrics are affected. Agent pending-state and credential-state persistence must retain acknowledged document hashes across restarts. No new external service is required, and legacy agents continue through the existing full Sync path.
