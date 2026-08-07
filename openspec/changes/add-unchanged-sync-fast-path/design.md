## Context

`POST /v1/sync` currently authenticates an endpoint and then performs capability persistence, release and artifact resolution, delivery acknowledgement, telemetry persistence, cron evaluation, remediation-policy lookup, upgrade/diagnostic/change-control lookup, delivery-state persistence, check-in persistence, and request audit persistence before returning an unchanged response. Several stores already avoid replacing equal content, but reaching those comparisons still costs database queries and transactions.

The optimization must preserve the pull architecture and the current success polling interval. It also must not hide revocation, policy, release, enrollment, targeting, scheduled work, diagnostics, upgrades, or execution authorization. A cache hit is therefore a proof that both the agent inputs and every server authority that can affect the response remain unchanged, not merely a lookup by artifact digest.

Remotr's documented production path runs on Fly Machines, which may stop when idle and may be replaced or scaled horizontally. A process-local cache remains useful for fixed single-process deployments, but it cannot preserve a warm decision across a Fly Machine stop/start and cannot coordinate invalidation between Machines. Fly's managed Redis offering is Upstash Redis, provisioned with `fly redis create` and reached through an authenticated organization-private primary URL.

The public verification seam is authenticated Sync. This is a time-, concurrency-, persistence-, authorization-, and hot-path change, so implementation requires deterministic clock tests, malformed and unauthorized protocol cases, mutation races, Postgres operation evidence, native allocation benchmarks, and the authenticated fleet load harness.

## Goals / Non-Goals

**Goals:**

- Serve eligible unchanged Sync responses from a selectable bounded memory or Redis backend with zero Postgres operations.
- Preserve valid shared decisions across Fly Machine replacement and coordinate multiple serving processes through Redis.
- Make stale cache use impossible after an in-scope authoritative mutation becomes observable.
- Elide unchanged agent documents without trusting an unverified client-supplied hash.
- Persist semantic changes immediately while bounding steady-state liveness and audit durability lag.
- Remain backward compatible with legacy agents and fail closed to the full Sync path whenever eligibility is uncertain.

**Non-Goals:**

- Adaptive success-poll backoff or a change to polling jitter, retry, or overload behavior.
- Caching artifact-bearing, capability-blocked, command-bearing, or telemetry-bearing responses.
- Replacing Postgres as durable endpoint, policy, release, or audit authority; Redis remains disposable acceleration and coordination state.
- Treating Redis Pub/Sub, asynchronous notifications, or read replicas as a correctness boundary.
- Changing artifact composition, provider execution, remediation semantics, or acknowledgement semantics.

## Decisions

### 1. Put the proven quiet decision behind a backend contract

Retain the existing bounded memory implementation behind an `UnchangedSyncBackend` contract and add a Redis implementation. The contract owns atomic decision lookup, conditional fill, scoped mutation begin/complete, invalidation, checkpoint observation/claim/complete, and bounded status. An entry is bound to authenticated endpoint ID and certificate fingerprint and contains:

- endpoint fleet/targeting identity and the authority generations used to compute it;
- the acknowledged release and artifact digest;
- accepted hashes for capability, system-information, targeting, and other repeatable agent documents;
- the immutable unchanged response projection, including policy and crons digest;
- the last durable liveness/audit checkpoint and the earliest time-dependent revalidation deadline; and
- size/accounting metadata for bounded LRU/TTL eviction.

Neither backend stores artifact bytes, secrets, raw inventory, or raw capability documents. Response serialization and compression remain outside the entry so request-specific headers are never reused. Redis values use a versioned bounded encoding; deployment namespace and hashed scope/endpoint keys prevent cross-environment collisions and raw endpoint identifiers in Redis key names.

A request is eligible only after mTLS identity extraction, bounded JSON/hash validation, and admission control, and only when all of the following hold: the certificate and endpoint identity match the entry; all authority generations match; release/delivery and accepted document hashes match; the request has no full documents, pending telemetry, results, acknowledgements, or intents; the cached response contains no artifact, block, due cron, diagnostic, upgrade, execution lease, or other one-shot work; and neither the checkpoint nor another revalidation deadline is due. A miss or doubt runs the existing full path.

This is preferred over general response caching because Sync contains state transitions and time-dependent commands. It is preferred over querying a cache-version table because that would retain a Postgres read on the hot path. The memory backend remains the lowest-latency choice for one stable process; Redis trades a network operation for restart reuse and shared correctness.

### 2. Select disabled, memory, or Redis mode explicitly

`REMOTR_UNCHANGED_SYNC_BACKEND` accepts `disabled`, `memory`, or `redis`. The existing `REMOTR_UNCHANGED_SYNC_FAST_PATH=false` remains the emergency rollback switch and overrides backend selection to disabled. Memory is the default outside the Fly bootstrap. Redis requires `REMOTR_REDIS_URL` and `REMOTR_UNCHANGED_SYNC_REDIS_PREFIX`; incomplete or conflicting configuration fails startup without logging the URL. Memory remains disabled when `REMOTR_SERVER_PROCESSES` exceeds one. Redis permits multiple processes sharing the same primary URL and namespace.

The Redis client uses the native Redis protocol with authentication, bounded dial/read/write timeouts, and no automatic command retries that could obscure mutation outcomes. `rediss://` is supported for external TLS endpoints. Fly's generated `redis://` private URL is accepted because it is password-authenticated and routed only over the Fly organization's private IPv6 network. Correctness operations always use the primary; Upstash read replicas are not used because stale generation reads would violate the barrier.

### 3. Use scoped authority generations with a mutation barrier

The invalidator maintains global, fleet, and endpoint generations. A mutation marks its affected scope as changing and invalidates matching decisions before durable work begins. While a scope is changing, it cannot return or accept cache entries. Completion advances the generation again and reopens the scope; a failed mutation also leaves old entries invalidated. Full Sync may continue during the mutation but cannot publish a cache entry from an unstable authority snapshot.

Invalidation is wired into the service boundary shared by every mutation path rather than added only to individual HTTP handlers. The minimum mapping is:

| Scope | Invalidating events |
| --- | --- |
| Global | release-ref advance/rollback, global delivery or change-control authority changes |
| Fleet | remediation policy, fleet artifact/cron target, fleet upgrade/diagnostic/lease changes |
| Endpoint | enrollment/register/delete/revoke/reassign, label/target changes, endpoint upgrade/diagnostic/lease changes |

Credential, enrollment/deployment token, secret, and change-request revocations invalidate conservatively at the narrowest provably safe scope. Mutation audit records are never coalesced.

Memory uses the existing mutex-protected generation barrier. Redis uses server-side Lua scripts against the primary so lookup plus generation/barrier validation, conditional fill, mutation begin, and mutation completion each have a single shared linearization point. Pub/Sub and Postgres `LISTEN/NOTIFY` are not correctness mechanisms because another process could serve stale data before receiving a notification. A Redis mutation begin must succeed before durable authority work starts. If completion fails after durable work, the begin barrier remains closed, forcing full Sync until a later bounded recovery reconciles and completes that scope.

### 4. Add acknowledged, domain-separated document hashes

The Sync request gains a bounded versioned document-hash summary. Hashes use lower-case `sha256:<hex>` over canonical, schema-versioned bytes with a document-type domain separator. The server recomputes every hash when a full document is present and rejects mismatches. Hashes are comparison hints, never authorization evidence.

The response reports accepted hashes and may request named full documents. An agent elides a document only after the server has acknowledged its hash, persists those acknowledgements in protected agent state, and resends the full document when its semantic hash changes or the server requests it. The existing release ref and artifact digest remain the delivery acknowledgement; their domain-separated pair is included in fast-path comparison without changing offer/active semantics.

Capability, system-information, labels/usernames or equivalent targeting inputs, and other repeatable documents use this mechanism. Event telemetry such as drift transitions, failures, cron results, diagnostics, firewall audit changes, reboot/network intents, and change preflights is never suppressed merely because a cache entry exists.

This explicit acknowledgement is preferred over assuming that a successful upload was persisted, because a lost response must not cause an agent to omit the only copy the server has accepted. On a cold cache, the full path may compare a supplied hash to durable content; if durable content is absent or cannot be validated, the response requests the full document and does not prime the fast path.

### 5. Separate semantic persistence from durability checkpoints

Persistence adapters expose changed-only operations for capability documents, delivery state, and system information. They compare canonical semantic hashes or explicit delivery fields and do not update timestamps or issue an `UPDATE` when content is equal. A real semantic change is persisted on the full Sync before its hash is acknowledged.

Successful quiet hits update only the selected backend's bounded observation accumulator. Memory retains the existing process-local behavior. Redis atomically increments the observation and, when due, grants one process a short checkpoint claim token; completion removes the claim only after `last_seen` and the aggregate audit checkpoint are durable. An expired claim is retryable by another process. The default interval is five minutes; configuration accepts five through ten minutes and rejects a larger value. A memory-process crash may lose only observations since the last checkpoint; Redis observations survive process replacement but remain disposable if Redis itself loses data. Security failures, malformed requests, overload decisions, state changes, commands, and administrative mutations keep their existing immediate audit behavior.

If a due checkpoint fails, the endpoint is not considered durably checkpointed and the entry cannot resume ordinary cache hits until a later full path completes the checkpoint. Clock behavior is injected; no test sleeps on wall time.

### 6. Bound Redis storage without relying on provider eviction

Redis stores decision values with TTL and maintains namespace-local LRU/byte metadata. Conditional fill uses one atomic script to reject oversized entries, evict the oldest bounded decisions until both configured budgets fit, and update counters without touching generation/barrier keys. The Fly bootstrap provisions Upstash with provider eviction disabled so random provider eviction cannot remove coordinator keys; Remotr's own bounds remain authoritative. Exhausted or rejected Redis writes simply prevent cache admission and never fail a completed Sync.

### 7. Derive expiry from all time-dependent work

Each entry's `validUntil` is the minimum of its liveness/audit checkpoint, cache TTL, next cron evaluation, and any known upgrade, diagnostic, change-control, reboot, network, or policy deadline. An entry is not primed when the full path cannot determine a safe deadline. Redis uses absolute UTC deadlines plus Redis TTL; the lookup script rejects an entry at the deadline even if physical expiry has not run. This makes a timer transition a normal cache miss without relying on wall-clock sleeps or asynchronous key expiry.

The server primes only from a successful full Sync whose response is quiet and whose durable writes have completed. Cold start, eviction, legacy protocol input, malformed hashes, missing documents, and persistence uncertainty all use the full path. Cache loss affects performance only.

### 8. Make the fast path observable without high-cardinality metrics

Expose bounded counters for backend, hit, miss by stable reason, eviction, invalidation by scope, document request/change, checkpoint, Redis error class, fallback, and fast-path-disabled state. Histograms cover decision latency and response bytes; Postgres instrumentation counts operations per Sync without endpoint identifiers. Logs and audit checkpoints contain no Redis URL, raw document, secret, certificate, endpoint key, or high-cardinality hash value.

The native benchmark uses a representative cached request and reports allocations. Authenticated load primes 400 distinct endpoints in both modes, measures the interval between checkpoints, and proves zero application Postgres operations for eligible hits. Redis scenarios use two server processes and separately cover process replacement, release/policy/revocation invalidation, Redis outage fallback, mutation rejection during outage, recovery, and bounded provider command volume.

### 9. Provision Fly Redis during bootstrap

The Fly bootstrap creates or reuses an Upstash database named from the Remotr app, in the selected Fly organization and primary region, using `fly redis create` with a pay-as-you-go plan, no read replicas, and provider eviction disabled. It reads the private primary URL from `fly redis status`, never prints it, and imports it as `REMOTR_REDIS_URL` alongside `REMOTR_UNCHANGED_SYNC_BACKEND=redis` and a deployment-unique prefix. `REMOTR_REDIS_URL` reuses an operator-supplied Redis service; `REMOTR_SKIP_REDIS=1` explicitly retains memory mode and documents that Fly stop/start makes the cache cold. Bootstrap remains idempotent and reports cost/cleanup guidance without destroying an existing database.

## Risks / Trade-offs

- **[Stale authorization or policy after a racing mutation]** → Begin scoped invalidation before durable mutation, prohibit fills during the unstable generation, and add deterministic race tests for every invalidation class.
- **[A response-affecting dependency is omitted from eligibility]** → Centralize a conservative eligibility predicate and invalidation registry; unknown work, unknown deadlines, or unsupported protocol versions always miss.
- **[Liveness/audit data is lost on crash]** → Use a five-minute default, enforce a ten-minute maximum, aggregate bounded checkpoints, and document that in-memory observations since the last checkpoint are intentionally lossy.
- **[Cache storage grows with fleet size]** → Enforce namespace-local entry/byte budgets, TTL/LRU eviction, immutable small projections, and allocation/soak evidence in both modes.
- **[Hash collision or client lie suppresses content]** → Use domain-separated SHA-256, recompute hashes for full documents, bind entries to authenticated identity, and request content whenever durable evidence is absent.
- **[Redis outage or ambiguous command outcome]** → Sync falls back to Postgres, fills stop, mutations require a successful begin barrier, and an incomplete completion leaves the scope closed until reconciliation.
- **[Multi-process servers observe mutations at different times]** → Allow multi-process only through primary-backed atomic Redis scripts; memory mode remains single-process.
- **[Upstash cost scales with commands]** → Use one atomic operation per eligible lookup, bounded checkpoint cadence, no read replicas, documented pay-as-you-go cost, and command-count load evidence.
- **[Changed-only SQL still takes a read or no-op transaction]** → Keep it off the hit path and use operation-count tests to distinguish zero-operation cache hits from changed-only full-path writes.

## Migration Plan

1. Add the backward-compatible request/response hash fields, agent acknowledgement state, and changed-only persistence while the cache remains disabled.
2. Add invalidation generations and wire every response-affecting mutation; verify mutation races before allowing entries to be primed.
3. Retain the verified memory backend behind the new backend contract, then add Redis atomic lookup/fill/invalidation/checkpoint behavior with conservative limits and failure fallback.
4. Update Fly bootstrap to provision or reuse Upstash and set Redis mode; update deployment, environment, architecture, audit, and troubleshooting documentation before making Redis the Fly default.
5. Run focused authenticated Sync tests, Redis compatibility/integration checks, Postgres integration checks, native benchmarks, and the 400-endpoint memory/Redis unchanged, restart, fan-out, outage, and revocation scenarios.
6. Roll back with `REMOTR_UNCHANGED_SYNC_FAST_PATH=false` or `REMOTR_UNCHANGED_SYNC_BACKEND=disabled`. Durable Postgres state and legacy full Sync remain valid; Redis keys may expire unused. Memory mode remains available for stable single-process deployments.

## Open Questions

None. The Fly default is Redis-backed; memory remains the default for non-Fly configuration unless an operator selects otherwise. Exact internal type names may follow repository conventions, but the eligibility, acknowledgement, atomic invalidation, failure, durability, and evidence contracts above are fixed for implementation.
