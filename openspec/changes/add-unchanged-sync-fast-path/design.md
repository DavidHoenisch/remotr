## Context

`POST /v1/sync` currently authenticates an endpoint and then performs capability persistence, release and artifact resolution, delivery acknowledgement, telemetry persistence, cron evaluation, remediation-policy lookup, upgrade/diagnostic/change-control lookup, delivery-state persistence, check-in persistence, and request audit persistence before returning an unchanged response. Several stores already avoid replacing equal content, but reaching those comparisons still costs database queries and transactions.

The optimization must preserve the pull architecture and the current success polling interval. It also must not hide revocation, policy, release, enrollment, targeting, scheduled work, diagnostics, upgrades, or execution authorization. A cache hit is therefore a proof that both the agent inputs and every server authority that can affect the response remain unchanged, not merely a lookup by artifact digest.

The public verification seam is authenticated Sync. This is a time-, concurrency-, persistence-, authorization-, and hot-path change, so implementation requires deterministic clock tests, malformed and unauthorized protocol cases, mutation races, Postgres operation evidence, native allocation benchmarks, and the authenticated fleet load harness.

## Goals / Non-Goals

**Goals:**

- Serve eligible unchanged Sync responses from bounded process memory with zero database operations.
- Make stale cache use impossible after an in-scope authoritative mutation becomes observable.
- Elide unchanged agent documents without trusting an unverified client-supplied hash.
- Persist semantic changes immediately while bounding steady-state liveness and audit durability lag.
- Remain backward compatible with legacy agents and fail closed to the full Sync path whenever eligibility is uncertain.

**Non-Goals:**

- Adaptive success-poll backoff or a change to polling jitter, retry, or overload behavior.
- Caching artifact-bearing, capability-blocked, command-bearing, or telemetry-bearing responses.
- Replacing Postgres as durable state or making an in-memory entry authoritative after restart.
- Enabling the fast path across multiple server processes without a synchronous invalidation coordinator.
- Changing artifact composition, provider execution, remediation semantics, or acknowledgement semantics.

## Decisions

### 1. Cache a proven quiet Sync decision, not arbitrary response bytes

Introduce a bounded `UnchangedSyncCache` owned by `Server`. An entry is keyed by authenticated endpoint ID and certificate fingerprint and contains:

- endpoint fleet/targeting identity and the authority generations used to compute it;
- the acknowledged release and artifact digest;
- accepted hashes for capability, system-information, targeting, and other repeatable agent documents;
- the immutable unchanged response projection, including policy and crons digest;
- the last durable liveness/audit checkpoint and the earliest time-dependent revalidation deadline; and
- size/accounting metadata for bounded LRU/TTL eviction.

The cache stores no artifact bytes, secrets, raw inventory, or raw capability document. Response serialization and compression remain outside the entry so request-specific headers are never reused.

A request is eligible only after mTLS identity extraction, bounded JSON/hash validation, and admission control, and only when all of the following hold: the certificate and endpoint identity match the entry; all authority generations match; release/delivery and accepted document hashes match; the request has no full documents, pending telemetry, results, acknowledgements, or intents; the cached response contains no artifact, block, due cron, diagnostic, upgrade, execution lease, or other one-shot work; and neither the checkpoint nor another revalidation deadline is due. A miss or doubt runs the existing full path.

This is preferred over general response caching because Sync contains state transitions and time-dependent commands. It is preferred over querying a cache-version table because that would retain a database read on the hot path.

### 2. Use scoped authority generations with a mutation barrier

The invalidator maintains global, fleet, and endpoint generations. A mutation marks its affected scope as changing and evicts matching entries before durable work begins. While a scope is changing, it cannot produce or accept cache entries. Completion advances the generation again and reopens the scope; a failed mutation also leaves old entries evicted. Full Sync may continue during the mutation but cannot publish a cache entry from an unstable authority snapshot.

Invalidation is wired into the service boundary shared by every mutation path rather than added only to individual HTTP handlers. The minimum mapping is:

| Scope | Invalidating events |
| --- | --- |
| Global | release-ref advance/rollback, global delivery or change-control authority changes |
| Fleet | remediation policy, fleet artifact/cron target, fleet upgrade/diagnostic/lease changes |
| Endpoint | enrollment/register/delete/revoke/reassign, label/target changes, endpoint upgrade/diagnostic/lease changes |

Credential, enrollment/deployment token, secret, and change-request revocations invalidate conservatively at the narrowest provably safe scope. Mutation audit records are never coalesced.

The first implementation enables this process-local fast path only in the supported single-server topology. If multiple serving processes are configured without a coordinator that provides the same begin/complete barrier semantics, startup rejects or disables the fast path and records that state. Postgres `LISTEN/NOTIFY` alone is not considered synchronous invalidation because another process could serve a stale entry between commit and notification.

### 3. Add acknowledged, domain-separated document hashes

The Sync request gains a bounded versioned document-hash summary. Hashes use lower-case `sha256:<hex>` over canonical, schema-versioned bytes with a document-type domain separator. The server recomputes every hash when a full document is present and rejects mismatches. Hashes are comparison hints, never authorization evidence.

The response reports accepted hashes and may request named full documents. An agent elides a document only after the server has acknowledged its hash, persists those acknowledgements in protected agent state, and resends the full document when its semantic hash changes or the server requests it. The existing release ref and artifact digest remain the delivery acknowledgement; their domain-separated pair is included in fast-path comparison without changing offer/active semantics.

Capability, system-information, labels/usernames or equivalent targeting inputs, and other repeatable documents use this mechanism. Event telemetry such as drift transitions, failures, cron results, diagnostics, firewall audit changes, reboot/network intents, and change preflights is never suppressed merely because a cache entry exists.

This explicit acknowledgement is preferred over assuming that a successful upload was persisted, because a lost response must not cause an agent to omit the only copy the server has accepted. On a cold cache, the full path may compare a supplied hash to durable content; if durable content is absent or cannot be validated, the response requests the full document and does not prime the fast path.

### 4. Separate semantic persistence from durability checkpoints

Persistence adapters expose changed-only operations for capability documents, delivery state, and system information. They compare canonical semantic hashes or explicit delivery fields and do not update timestamps or issue an `UPDATE` when content is equal. A real semantic change is persisted on the full Sync before its hash is acknowledged.

Successful quiet hits update only an in-memory observation accumulator. The first eligible Sync at or after the checkpoint deadline bypasses the fast path and durably writes one `last_seen` checkpoint plus one aggregate successful-Sync audit checkpoint containing a bounded window start/end and count, then re-primes the entry. The default interval is five minutes; configuration accepts five through ten minutes and rejects a larger value. A crash may lose only the observations since the last completed checkpoint. Security failures, malformed requests, overload decisions, state changes, commands, and administrative mutations keep their existing immediate audit behavior.

If a due checkpoint fails, the endpoint is not considered durably checkpointed and the entry cannot resume ordinary cache hits until a later full path completes the checkpoint. Clock behavior is injected; no test sleeps on wall time.

### 5. Derive expiry from all time-dependent work

Each entry's `validUntil` is the minimum of its liveness/audit checkpoint, cache TTL, next cron evaluation, and any known upgrade, diagnostic, change-control, reboot, network, or policy deadline. An entry is not primed when the full path cannot determine a safe deadline. This makes a timer transition a normal cache miss even without an explicit mutation.

The server primes only from a successful full Sync whose response is quiet and whose durable writes have completed. Cold start, eviction, legacy protocol input, malformed hashes, missing documents, and persistence uncertainty all use the full path. Cache loss affects performance only.

### 6. Make the fast path observable without high-cardinality metrics

Expose bounded counters for hit, miss by stable reason, eviction, invalidation by scope, document request/change, checkpoint, and fast-path-disabled state. Histograms cover decision latency and response bytes; Postgres instrumentation counts operations per Sync without endpoint identifiers. Logs and audit checkpoints contain no raw document, secret, certificate, or high-cardinality hash values.

The native benchmark uses a representative cached request and reports allocations. Authenticated load primes 400 distinct endpoints, measures the interval between checkpoints, and proves zero application database operations for eligible hits while still observing immediate release, policy, and revocation invalidation.

## Risks / Trade-offs

- **[Stale authorization or policy after a racing mutation]** → Begin scoped invalidation before durable mutation, prohibit fills during the unstable generation, and add deterministic race tests for every invalidation class.
- **[A response-affecting dependency is omitted from eligibility]** → Centralize a conservative eligibility predicate and invalidation registry; unknown work, unknown deadlines, or unsupported protocol versions always miss.
- **[Liveness/audit data is lost on crash]** → Use a five-minute default, enforce a ten-minute maximum, aggregate bounded checkpoints, and document that in-memory observations since the last checkpoint are intentionally lossy.
- **[Cache memory grows with fleet size]** → Enforce entry-count and byte budgets, TTL/LRU eviction, immutable small projections, and allocation/soak evidence.
- **[Hash collision or client lie suppresses content]** → Use domain-separated SHA-256, recompute hashes for full documents, bind entries to authenticated identity, and request content whenever durable evidence is absent.
- **[Multi-process servers observe mutations at different times]** → Disable the fast path unless a synchronous cross-process invalidation coordinator is configured and verified.
- **[Changed-only SQL still takes a read or no-op transaction]** → Keep it off the hit path and use operation-count tests to distinguish zero-operation cache hits from changed-only full-path writes.

## Migration Plan

1. Add the backward-compatible request/response hash fields, agent acknowledgement state, and changed-only persistence while the cache remains disabled.
2. Add invalidation generations and wire every response-affecting mutation; verify mutation races before allowing entries to be primed.
3. Enable the cache behind server configuration with conservative limits, a five-minute checkpoint, and metrics. Legacy and hash-incomplete requests remain on the full path.
4. Run focused authenticated Sync tests, Postgres integration checks, the native benchmark, and the 400-endpoint unchanged/fan-out/revocation load scenarios before enabling it by default.
5. Roll back by disabling cache admission. Durable Postgres state and legacy full Sync remain valid; newly added wire fields are optional and agent acknowledgement state may remain unused.

## Open Questions

None. Exact internal type and wire-field names may follow repository conventions, but the eligibility, acknowledgement, invalidation, durability, and evidence contracts above are fixed for implementation.
