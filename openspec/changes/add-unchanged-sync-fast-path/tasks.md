## 1. Protocol and Evidence Foundation

- [ ] 1.1 Record the pre-change `make test`, authenticated unchanged-Sync load, database-operation, payload, and allocation baselines; map OS-USF-001 through OS-USF-010 and OS-PSA-004/015/016/017 to the authenticated Sync, configuration, Postgres, benchmark, and load evidence layers before production edits.
- [ ] 1.2 Complete the OS-USF-003 red → green slice at authenticated Sync: add one focused malformed/mismatched canonical document-hash test, record the intended red failure, implement bounded domain-separated hash parsing/recomputation and optional request/response fields, rerun green, then add boundary/table cases and a bounded fuzz property for hash-summary decoding.
- [ ] 1.3 Complete the OS-USF-004 red → green slice at the agent Sync client seam: prove an acknowledged unchanged capability/system-information document is represented by hashes only, implement persisted accepted-hash state plus full-document requests, rerun green, then cover changed content, lost responses, and agent restart.
- [ ] 1.4 Complete the OS-USF-008 compatibility slice by recording a red expectation for hash-ineligible legacy behavior where needed, implementing backward-compatible fallback, and rerunning the legacy authenticated Sync fixture plus malformed, unauthenticated, and unknown-endpoint regressions.

## 2. Changed-Only Durable State

- [ ] 2.1 Complete the OS-USF-005 capability slice against the Postgres persistence seam: prove equal canonical capability content issues no update, implement the minimum changed-only operation, rerun green, then cover changed content and persistence failure.
- [ ] 2.2 Complete the OS-USF-005 system-information slice against the same seam: prove an equal digest/document issues no update, implement changed-only persistence, rerun green, then cover changed inventory and hash mismatch.
- [ ] 2.3 Complete the OS-USF-005 delivery slice: prove repeated target/offer/active fields do not change solely because timestamps advance, implement semantic comparison/upsert behavior, rerun green, then cover a genuine acknowledgement transition.
- [ ] 2.4 Complete the acknowledgement-ordering slice at authenticated Sync: make persistence fail after a new full document or delivery acknowledgement, record that the server must not accept its hash, implement durable-before-ack ordering, and rerun the focused test plus Postgres integration regressions.

## 3. Bounded Quiet-Decision Cache

- [ ] 3.1 Complete the OS-USF-001 red → green slice at authenticated Sync with instrumented persistence boundaries: prime one quiet endpoint, prove the repeated request currently touches storage, implement the smallest authenticated in-memory decision path, and rerun until the request returns unchanged with exactly zero database operations.
- [ ] 3.2 Complete the OS-USF-002 eligibility slice with a table of independently expected bypass cases for full documents, telemetry, results, intents, one-shot response work, missing hashes, changed release/delivery, and unknown authority; implement the centralized conservative predicate and rerun green.
- [ ] 3.3 Complete the OS-USF-002 time slice with injected clock/schedule boundaries: prove a cron, diagnostic, upgrade, lease, or revalidation deadline forces the first due request onto the full path without wall-clock sleeps, implement `validUntil` derivation, and rerun green.
- [ ] 3.4 Complete the OS-USF-009 resource-bound slice: prove deterministic entry-count, byte, TTL, and observation limits at the Sync seam, implement bounded LRU/TTL eviction without retaining raw documents or artifact bytes, rerun green, and add soak/growth assertions.
- [ ] 3.5 Complete the OS-USF-009 topology slice through server configuration: prove uncoordinated multi-process mode cannot enable the process-local cache, implement fail-closed disable/rejection and status reporting, and rerun configuration boundary tests.

## 4. Immediate Mutation Invalidation

- [ ] 4.1 Complete the OS-USF-006 concurrency foundation slice with a deterministic blocked mutation and concurrent authenticated Sync: record the stale-fill/hit failure, implement global/fleet/endpoint generation barriers that evict before mutation and prohibit fills while unstable, and rerun under the race detector.
- [ ] 4.2 Complete the OS-USF-006 release slice through the public Git Sync/Admin API: prove release advance and rollback invalidate cached decisions before new release state is served, wire global invalidation at the shared release mutation boundary, and rerun green plus release fan-out regressions.
- [ ] 4.3 Complete the OS-USF-006 fleet slice through public mutation seams: prove remediation-policy and fleet artifact/cron targeting changes cannot return old cached responses, wire fleet invalidation, and rerun green including failed-mutation behavior.
- [ ] 4.4 Complete the OS-USF-006 endpoint slice through enrollment and Admin APIs: prove register, delete/revoke, reassignment, credential, and label/target changes invalidate only the required endpoint decisions, wire endpoint invalidation, and rerun green including a revocation race and unrelated-endpoint regression.
- [ ] 4.5 Audit every remaining response-affecting mutation service (upgrade, diagnostics, schedules, change authorization/leases, deployment/enrollment tokens, and secret/change-request revocations); for each uncovered class, complete one focused red → green public-seam invalidation slice and update the centralized scope registry.

## 5. Liveness and Audit Checkpoints

- [ ] 5.1 Complete the OS-USF-007 configuration slice with table tests for the five-minute default and accepted five-through-ten-minute range, record rejection outside the range, implement injected-clock checkpoint policy, and rerun green.
- [ ] 5.2 Complete the OS-USF-007 steady-window slice at authenticated Sync: prove repeated eligible polls write neither `last_seen` nor audit events, implement the bounded in-memory accumulator and one due liveness plus aggregate audit checkpoint, and rerun green with exact window/count assertions.
- [ ] 5.3 Complete the OS-USF-007 security slice: prove malformed, unauthorized, revoked, overloaded, state-changing, command-bearing, and administrative requests retain immediate classified audit behavior, implement the audit bypass/flush distinction, and rerun redaction canary regressions.
- [ ] 5.4 Complete the OS-USF-007 failure/restart slice: inject checkpoint persistence failure and process restart, prove the cache stays ineligible until a durable checkpoint/full Sync succeeds and cold start cannot reuse memory, implement recovery behavior, and rerun green without wall-clock sleeps.

## 6. Metrics, Benchmarks, and Authenticated Load

- [ ] 6.1 Complete the OS-USF-010 observability slice: record missing hit/miss-reason, invalidation, eviction, document, checkpoint, disabled-state, latency, bytes, and database-operation signals; implement bounded metrics/logs and verify no endpoint IDs, hashes, certificate material, raw documents, or secrets become labels or diagnostic content.
- [ ] 6.2 Add the native unchanged-Sync decision benchmark with a representative hash-only request, allocation reporting, cache-hit/miss fixtures, and versioned performance budget evidence; compare repeated controlled samples to the baseline rather than relying on one timing.
- [ ] 6.3 Complete OS-PSA-004 and OS-PSA-015 in the authenticated load harness: prime 400 distinct identities with full documents, run hash-only unchanged polling, attribute database operations by phase, prove zero operations per eligible hit, and prove bounded coalesced writes at checkpoint turnover.
- [ ] 6.4 Complete OS-PSA-016 and OS-PSA-017 load slices for release fan-out, policy mutation, endpoint revocation, cold restart, and eviction; verify the invalidation boundary, recovery to the fast path, error rate, hit ratio, payload/network changes, and bounded memory/goroutine/connection/database-row growth.

## 7. Documentation and Final Verification

- [ ] 7.1 Update the Sync API reference, architecture explanation, environment/configuration reference, audit guide, and production deployment guidance with optional hash fields, full-document requests, eligibility, checkpoint durability, invalidation scope, legacy behavior, rollback switch, and the single-process/coordinator constraint.
- [ ] 7.2 Update OpenSpec verification traceability and performance budget fixtures for OS-USF-001 through OS-USF-010 and OS-PSA-004/015/016/017; add no skips or manual exceptions unless an expiring reviewed entry is recorded in `test/evidence-exceptions.yaml`.
- [ ] 7.3 Run the focused authenticated Sync and Postgres suites, race tests, hash fuzz target, native benchmarks with allocation reporting, and relevant authenticated load scenarios; resolve failures, then run `make test` and record the final commands/results before handoff.
