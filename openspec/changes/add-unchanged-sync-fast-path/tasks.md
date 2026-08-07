## 1. Protocol and Evidence Foundation

- [x] 1.1 Record the pre-change `make test`, authenticated unchanged-Sync load, database-operation, payload, and allocation baselines; map OS-USF-001 through OS-USF-010 and OS-PSA-004/015/016/017 to the authenticated Sync, configuration, Postgres, benchmark, and load evidence layers before production edits.
- [x] 1.2 Complete the OS-USF-003 red → green slice at authenticated Sync: add one focused malformed/mismatched canonical document-hash test, record the intended red failure, implement bounded domain-separated hash parsing/recomputation and optional request/response fields, rerun green, then add boundary/table cases and a bounded fuzz property for hash-summary decoding.
- [x] 1.3 Complete the OS-USF-004 red → green slice at the agent Sync client seam: prove an acknowledged unchanged capability/system-information document is represented by hashes only, implement persisted accepted-hash state plus full-document requests, rerun green, then cover changed content, lost responses, and agent restart.
- [x] 1.4 Complete the OS-USF-008 compatibility slice by recording a red expectation for hash-ineligible legacy behavior where needed, implementing backward-compatible fallback, and rerunning the legacy authenticated Sync fixture plus malformed, unauthenticated, and unknown-endpoint regressions.

## 2. Changed-Only Durable State

- [x] 2.1 Complete the OS-USF-005 capability slice against the Postgres persistence seam: prove equal canonical capability content issues no update, implement the minimum changed-only operation, rerun green, then cover changed content and persistence failure.
- [x] 2.2 Complete the OS-USF-005 system-information slice against the same seam: prove an equal digest/document issues no update, implement changed-only persistence, rerun green, then cover changed inventory and hash mismatch.
- [x] 2.3 Complete the OS-USF-005 delivery slice: prove repeated target/offer/active fields do not change solely because timestamps advance, implement semantic comparison/upsert behavior, rerun green, then cover a genuine acknowledgement transition.
- [x] 2.4 Complete the acknowledgement-ordering slice at authenticated Sync: make persistence fail after a new full document or delivery acknowledgement, record that the server must not accept its hash, implement durable-before-ack ordering, and rerun the focused test plus Postgres integration regressions.

## 3. Bounded Quiet-Decision Cache

- [x] 3.1 Complete the OS-USF-001 red → green slice at authenticated Sync with instrumented persistence boundaries: prime one quiet endpoint, prove the repeated request currently touches storage, implement the smallest authenticated in-memory decision path, and rerun until the request returns unchanged with exactly zero database operations.
- [x] 3.2 Complete the OS-USF-002 eligibility slice with a table of independently expected bypass cases for full documents, telemetry, results, intents, one-shot response work, missing hashes, changed release/delivery, and unknown authority; implement the centralized conservative predicate and rerun green.
- [x] 3.3 Complete the OS-USF-002 time slice with injected clock/schedule boundaries: prove a cron, diagnostic, upgrade, lease, or revalidation deadline forces the first due request onto the full path without wall-clock sleeps, implement `validUntil` derivation, and rerun green.
- [x] 3.4 Complete the OS-USF-009 resource-bound slice: prove deterministic entry-count, byte, TTL, and observation limits at the Sync seam, implement bounded LRU/TTL eviction without retaining raw documents or artifact bytes, rerun green, and add soak/growth assertions.
- [x] 3.5 Complete the OS-USF-009 topology slice through server configuration: prove uncoordinated multi-process mode cannot enable the process-local cache, implement fail-closed disable/rejection and status reporting, and rerun configuration boundary tests.

## 4. Immediate Mutation Invalidation

- [x] 4.1 Complete the OS-USF-006 concurrency foundation slice with a deterministic blocked mutation and concurrent authenticated Sync: record the stale-fill/hit failure, implement global/fleet/endpoint generation barriers that evict before mutation and prohibit fills while unstable, and rerun under the race detector.
- [x] 4.2 Complete the OS-USF-006 release slice through the public Git Sync/Admin API: prove release advance and rollback invalidate cached decisions before new release state is served, wire global invalidation at the shared release mutation boundary, and rerun green plus release fan-out regressions.
- [x] 4.3 Complete the OS-USF-006 fleet slice through public mutation seams: prove remediation-policy and fleet artifact/cron targeting changes cannot return old cached responses, wire fleet invalidation, and rerun green including failed-mutation behavior.
- [x] 4.4 Complete the OS-USF-006 endpoint slice through enrollment and Admin APIs: prove register, delete/revoke, reassignment, credential, and label/target changes invalidate only the required endpoint decisions, wire endpoint invalidation, and rerun green including a revocation race and unrelated-endpoint regression.
- [x] 4.5 Audit every remaining response-affecting mutation service (upgrade, diagnostics, schedules, change authorization/leases, deployment/enrollment tokens, and secret/change-request revocations); for each uncovered class, complete one focused red → green public-seam invalidation slice and update the centralized scope registry.

## 5. Liveness and Audit Checkpoints

- [x] 5.1 Complete the OS-USF-007 configuration slice with table tests for the five-minute default and accepted five-through-ten-minute range, record rejection outside the range, implement injected-clock checkpoint policy, and rerun green.
- [x] 5.2 Complete the OS-USF-007 steady-window slice at authenticated Sync: prove repeated eligible polls write neither `last_seen` nor audit events, implement the bounded in-memory accumulator and one due liveness plus aggregate audit checkpoint, and rerun green with exact window/count assertions.
- [x] 5.3 Complete the OS-USF-007 security slice: prove malformed, unauthorized, revoked, overloaded, state-changing, command-bearing, and administrative requests retain immediate classified audit behavior, implement the audit bypass/flush distinction, and rerun redaction canary regressions.
- [x] 5.4 Complete the OS-USF-007 failure/restart slice: inject checkpoint persistence failure and process restart, prove the cache stays ineligible until a durable checkpoint/full Sync succeeds and cold start cannot reuse memory, implement recovery behavior, and rerun green without wall-clock sleeps.

## 6. Metrics, Benchmarks, and Authenticated Load

- [x] 6.1 Complete the OS-USF-010 observability slice: record missing hit/miss-reason, invalidation, eviction, document, checkpoint, disabled-state, latency, bytes, and database-operation signals; implement bounded metrics/logs and verify no endpoint IDs, hashes, certificate material, raw documents, or secrets become labels or diagnostic content.
- [x] 6.2 Add the native unchanged-Sync decision benchmark with a representative hash-only request, allocation reporting, cache-hit/miss fixtures, and versioned performance budget evidence; compare repeated controlled samples to the baseline rather than relying on one timing.
- [x] 6.3 Complete OS-PSA-004 and OS-PSA-015 in the authenticated load harness: prime 400 distinct identities with full documents, run hash-only unchanged polling, attribute database operations by phase, prove zero operations per eligible hit, and prove bounded coalesced writes at checkpoint turnover.
- [x] 6.4 Complete OS-PSA-016 and OS-PSA-017 load slices for release fan-out, policy mutation, endpoint revocation, cold restart, and eviction; verify the invalidation boundary, recovery to the fast path, error rate, hit ratio, payload/network changes, and bounded memory/goroutine/connection/database-row growth.

## 7. Documentation and Final Verification

- [x] 7.1 Update the Sync API reference, architecture explanation, environment/configuration reference, audit guide, and production deployment guidance with optional hash fields, full-document requests, eligibility, checkpoint durability, invalidation scope, legacy behavior, rollback switch, and the single-process/coordinator constraint.
- [x] 7.2 Update OpenSpec verification traceability and performance budget fixtures for OS-USF-001 through OS-USF-010 and OS-PSA-004/015/016/017; add no skips or manual exceptions unless an expiring reviewed entry is recorded in `test/evidence-exceptions.yaml`.
- [x] 7.3 Run the focused authenticated Sync and Postgres suites, race tests, hash fuzz target, native benchmarks with allocation reporting, and relevant authenticated load scenarios; resolve failures, then run `make test` and record the final commands/results before handoff.

## 8. Completion Audit Corrections

- [x] 8.1 Reopen OS-USF-004 after the completion audit found that production agents emitted only capability and system-information hashes; record the red wire failure, implement canonical delivery and targeting hashes, acknowledged elision, changed-content resend, server-request resend, lost-response safety, and restart restoration.
- [x] 8.2 Reopen OS-USF-003/005 after the completion audit found that delivery and targeting hashes were not recomputed or durably acknowledged by authenticated Sync; add mismatch/no-persistence and durable-before-ack tests, then implement fail-closed validation and acceptance ordering.
- [x] 8.3 Prove changed-only targeting persistence against real Postgres and rerun the representative four-domain fast-path/load evidence.
- [x] 8.4 Update traceability, API/evidence records, rerun focused race/fuzz/benchmark gates and `make test`, and reconcile the final OpenSpec task state.

## 9. Selectable Memory and Redis Backends

- [x] 9.1 Reopen the verification map for OS-USF-011 and OS-PSA-018; record Redis command, latency, allocation, connection, and load baselines, and select authenticated Sync, configuration, real-Redis integration, multi-process load, secret-redaction, and Fly bootstrap evidence before production edits.
- [x] 9.2 Complete the backend-selection red → green slice through the public server configuration seam: prove `disabled`, `memory`, and `redis` select the expected behavior; implement the backend contract and compatibility mapping for the legacy enable flag; then cover missing/invalid Redis URL, namespace validation, multi-process memory rejection, rollback, and secret-canary redaction cases.
- [x] 9.3 Complete the Redis decision-cache red → green slice against a real Redis-compatible service: prove a primed hash-only authenticated Sync survives server-process replacement without Postgres work; implement versioned, namespaced, bounded lookup/fill state with atomic Lua operations; then cover identity isolation, stale generations, TTL, LRU/byte bounds, malformed values, primary-only operation, and deterministic connection failures.
- [x] 9.4 Complete the shared mutation-barrier red → green slice with two server processes and a deterministically blocked public mutation: prove a concurrent Sync cannot hit or refill stale state; implement atomic global/fleet/endpoint generation transitions before durable mutation; then cover success, rollback, script failure, Redis outage, revocation races, and the requirement that an unavailable barrier rejects the mutation before any durable write.
- [x] 9.5 Complete the Redis checkpoint red → green slice at authenticated Sync: prove concurrent processes claim at most one due liveness/audit checkpoint and a replacement process can reuse a valid durable claim; implement atomic claim/complete/retry-expiry state; then cover persistence failure, abandoned claims, checkpoint expiry, Redis loss, and conservative full-Postgres fallback without wall-clock sleeps.
- [x] 9.6 Complete Fly bootstrap red → green script tests for create, reuse, externally supplied URL, explicit skip, rerun idempotency, partial failure, and secret-free output; implement creation or discovery of an Upstash Redis database with `fly redis create`/`fly redis status`, import its private URL and Redis backend settings as Fly secrets, disable provider eviction where supported, and emit clear cost, rollback, and cleanup guidance without printing credentials.
- [x] 9.7 Update the Sync API reference, architecture explanation, environment/configuration reference, production and audit guides, troubleshooting material, and Fly deployment README for backend selection, Redis security and failure semantics, namespace isolation, primary-only/Lua requirements, memory persistence limitations, migration/rollback, Upstash costs, bootstrap opt-out/reuse, and resource cleanup.
- [x] 9.8 Complete OS-PSA-018 in the authenticated load harness with at least two server processes and 400 distinct endpoint identities: compare memory and Redis modes, replace a server during steady polling, exercise Redis interruption/recovery and mutation fan-out, and verify database operations, Redis commands, hit ratio, checkpoint coalescing, latency, error rate, and bounded memory/goroutine/connection/key growth against recorded budgets.
- [x] 9.9 Run focused configuration, authenticated Sync, mutation, checkpoint, Fly script, real-Redis integration, race, fuzz, benchmark, and multi-process load checks; update traceability and evidence records without unreviewed skips, then run `make test` and reconcile the final OpenSpec task state.
