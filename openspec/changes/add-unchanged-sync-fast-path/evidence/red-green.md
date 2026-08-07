# Red/green evidence

## OS-USF-003 — canonical document hashes

- Public seam: authenticated `POST /v1/sync`.
- Focused test:
  `go test -mod=vendor ./internal/server -run '^TestSyncRejectsMismatchedCapabilityDocumentHashWithoutPersistence$' -count=1`
- Intended red observed 2026-08-07: FAIL. The server returned HTTP 200 with
  an artifact for a full capability document accompanied by a false declared
  `sha256:` document hash; expected HTTP 400 and no persistence.
- Focused green: PASS after strict bounded summary decoding, a versioned
  capability-domain hash, recomputation before persistence, and response
  acknowledgement after successful persistence.
- Boundary/table and fuzz seed evidence: PASS for unsupported versions, empty
  sets, unknown names/fields, uppercase/short hashes, trailing JSON, oversize
  input, all four supported domains, the fixed domain-separation vector, and
  `FuzzDecodeSummaryBoundedRoundTrip`.
- Regression evidence: the focused capability identity, modern omission,
  mismatch, and accepted-hash server cases all pass.

## OS-USF-004 — agent document acknowledgement

- Public seam: consecutive agent Sync exchanges plus protected credential
  state across construction of a restarted agent.
- Focused test:
  `go test -mod=vendor ./cmd/remotr-agent -run '^TestSyncRunElidesAcknowledgedDocumentsAndRestoresHashesAfterRestart$' -count=1`
- Intended red observed 2026-08-07: FAIL. The first emitted Sync request omitted
  `documentHashes`; the test server closed the exchange and the client received
  EOF.
- Focused green: PASS. The first request sends full capability and system
  information with their hashes; acknowledged follow-ups and a reconstructed
  agent send hashes only from protected credential state.
- Regression evidence: PASS for lost-response retransmission, changed
  capability, changed system information, explicit full-document request,
  accepted-hash credential round-trip, and cold server rejection/request of an
  unknown hash-only capability without persistence.

## OS-USF-008 — legacy compatibility

- Public seam: frozen authenticated schema-0 Sync fixture.
- Compatibility expectation: the existing legacy request remains valid and is
  never treated as hash-protocol eligible; it receives neither accepted hashes
  nor document requests.
- Result: PASS for schema-0 delivery and acknowledgement, exact known-legacy
  mapping, unknown-legacy minimal fallback, malformed request, unauthenticated
  request, unknown authenticated endpoint, and missing endpoint identity.
- No legacy production branch was added: absence of the optional hash fields
  continues through the pre-existing authenticated full Sync path.

## OS-USF-005 — changed-only capability persistence

- Public seam: capability-document Postgres persistence contract.
- Focused test:
  `go test -mod=vendor ./internal/store/postgres -run '^TestCapabilityDocumentPersistenceRejectsDigestCanonicalMismatchBeforeUpdate$' -count=1`
- Intended red observed 2026-08-07: FAIL. A record retaining an existing digest
  but replacing its canonical body returned `changed=false, err=nil`; the store
  had not independently verified the digest/body invariant before classifying
  it as unchanged.
- Focused green: PASS after validating canonical bytes against the indexed
  digest before issuing SQL.
- Persistence cases: PASS for unchanged content, changed semantic content, and
  injected persistence failure.
- Real Postgres evidence: PASS with a temporary table shadowing the production
  schema; equal content returned `changed=false`, retained the original
  `received_at`, and retained the row's `xmin`, proving no row update occurred.

## OS-USF-005 — changed-only system-information persistence

- Public seam: system-information Postgres persistence contract.
- Focused test:
  `go test -mod=vendor ./internal/store/postgres -run '^TestSystemInformationPersistenceSkipsEqualSemanticDocument$' -count=1`
- Intended red observed 2026-08-07: BUILD FAIL. The persistence API returned
  only `error`, exposed no semantic changed/no-change result, and its fake
  boundary retained no operation evidence.
- Focused green: PASS with canonical semantic JSON, digest recomputation,
  changed/no-change reporting, and a conditional Postgres upsert.
- Regression evidence: PASS for formatting-equivalent JSON, changed inventory,
  hash mismatch, and injected persistence failure.
- Real Postgres evidence: PASS; an equal semantic document retained both row
  `xmin` and `reported_at`, proving no update occurred.

## OS-USF-005 — semantic delivery persistence

- Public seam: endpoint delivery-state persistence contract.
- Focused test:
  `go test -mod=vendor ./internal/store/postgres -run '^TestEndpointDeliveryStateSkipsTimestampOnlyReplay$' -count=1`
- Intended red observed 2026-08-07: BUILD FAIL. Delivery persistence returned
  only `error`, so callers could not distinguish a semantic transition from a
  timestamp-only replay.
- Focused green: PASS with changed/no-change reporting and a conditional SQL
  comparison that excludes offer, active, and update timestamps.
- Regression evidence: PASS for a genuine offered-to-active acknowledgement
  transition.
- Real Postgres evidence: PASS; replaying equal delivery fields with later
  timestamps retained row `xmin` and `updated_at`.

## Durable-before-ack document ordering

- Public seam: authenticated Sync with a valid full system-information
  document and an injected persistence failure.
- Focused test:
  `go test -mod=vendor ./internal/server -run '^TestSyncDoesNotAcknowledgeSystemInformationBeforeDurablePersistence$' -count=1`
- Intended red observed 2026-08-07: FAIL. The persistence error was logged and
  processing continued until a later artifact lookup returned HTTP 500;
  required behavior is an immediate HTTP 503 with no accepted hash.
- Focused green: PASS after validated system-information persistence moved
  ahead of response construction and accepted hashes became derived only from
  an internal durable-acceptance set.
- Ordering regressions: PASS for capability persistence failure,
  system-information persistence failure, and offered-to-active delivery
  persistence failure; none emits an accepted hash or mutates the durable
  fixture before success.
- All three real Postgres changed-only regressions remain green.

## OS-USF-001 — database-free unchanged Sync

- Public seam: authenticated Sync with a registry wrapper counting endpoint,
  capability, and delivery persistence operations.
- Focused test:
  `go test -mod=vendor ./internal/server -run '^TestEligibleUnchangedSyncPerformsZeroStorageOperations$' -count=1`
- Intended red observed 2026-08-07: BUILD FAIL. No fast-path configuration or
  decision cache contract existed.
- Focused green: PASS. A completed quiet full Sync primes a certificate-bound
  decision; the repeated hash-only request returns the unchanged response and
  the instrumented storage-operation counter remains exactly unchanged.

## OS-USF-002 — fail-closed eligibility

- Public seam: the centralized predicate used immediately before the
  authenticated cache lookup, anchored by the OS-USF-001 end-to-end hit.
- Result: PASS for full capability/system documents, labels, usernames, drift,
  apply failure, cron result, diagnostic result, firewall audit, preflight,
  reboot/network intents, upgrade status, missing hashes, changed delivery
  identity, certificate mismatch, and every command/document-request-bearing
  response class.

## OS-USF-002 — time boundary

- Public seam: authenticated hash-only Sync at the entry revalidation
  `validUntil`, driven by the injected server clock.
- Result: PASS. A request immediately before the deadline remains eligible;
  the first request exactly at the deadline bypasses memory and increments the
  instrumented persistence boundary, with no wall-clock sleep.
## OS-USF-009 bounded cache resources

- Public seam: authenticated Sync decision cache and its server-owned resource
  boundary.
- Selected layers: deterministic unit boundary plus authenticated Sync cache
  regressions; no wall-clock sleeps.
- Red: `go test -mod=vendor ./internal/server -run
  TestFastPathResourceBoundsDeterministic -count=1` failed because eight entries
  retained eight observations despite a configured cache-wide limit of three.
- Green: the cache now accounts observations globally and releases their count
  on eviction. The focused resource test and the authenticated zero-storage,
  fail-closed eligibility, and injected-clock deadline regressions pass.
- Boundaries: deterministic LRU order, exact TTL expiry, byte eviction,
  cache-wide observation cap, and 1,000 put/hit soak growth are asserted. Cache
  entries retain only identity/delivery/hash decision data and a response that
  has already been proven to contain no artifact bytes or one-shot work.
- Final bound audit red: the focused `pending_checkpoints` subtest reported
  `entries 0 bytes 0` after two live entries became failed/pending durability
  checkpoints, proving that pending state was outside the configured totals.
  Green: live decisions and pending checkpoints now share the same entry and
  byte budgets, and every completion or invalidation releases both accounting
  dimensions. `TestFastPathResourceBoundsDeterministic` passes in full.

## OS-USF-009 serving topology

- Public seam: `remotr-server` environment configuration and reported server
  fast-path status.
- Red: the focused server test did not compile because serving-process topology
  and status were absent; the command configuration test likewise failed with
  an undefined environment parser.
- Green: `REMOTR_UNCHANGED_SYNC_FAST_PATH` and the positive integer
  `REMOTR_SERVER_PROCESSES` are parsed at startup. A configured count above one
  retains a stable fail-closed status reason and never constructs a cache; the
  startup log reports only bounded `enabled` and `reason` values. Invalid
  booleans and non-positive process counts are rejected.
- Commands: focused `./cmd/remotr-server` configuration and `./internal/server`
  topology/resource tests both pass.

## OS-USF-006 mutation generation foundation

- Public seam: authenticated Sync held against an endpoint mutation barrier;
  supporting deterministic cache boundary test covers stale fills explicitly.
- Red: the focused test failed to compile before global/fleet/endpoint
  generation snapshots, begin/complete barriers, and snapshot-bound fills
  existed.
- Green: mutation begin increments the scoped generation and evicts before
  mutation work; all overlapping scopes remain unstable until completion.
  Cache hits validate current authority and a full-path response can fill only
  with the stable snapshot captured after endpoint lookup. A request held in
  the endpoint barrier performed storage work, could not refill, requested its
  full document after completion, and only then returned to zero-operation
  hits.
- Race evidence: focused authenticated Sync and stale-fill tests pass with
  `go test -race`.

## OS-USF-006 global release invalidation

- Public seam: authenticated Admin Git Sync; shared seam: `GitSyncer.Sync`, used
  by Admin, webhook, and polling paths.
- Red: advance, rollback, and failure table cases observed one cached decision
  when the configured Git mutation callback began. The shared Git Sync test did
  not compile before its begin/complete hook existed.
- Green: Admin begins global invalidation before invoking any configured sync,
  while `GitSyncer.Sync` also wraps the common composer/release-ref mutation so
  webhook and poll paths receive the same barrier. Initial startup sync runs
  before the cold cache is constructed; polling begins after the hook is wired.
  Completion is deferred on success and error, while failed mutations remain
  invalidated by the already-incremented generation.
- Focused Git Sync fan-out, provider-diagnostic, redaction, server
  configuration, and Admin invalidation regressions pass.

## OS-USF-006 fleet invalidation

- Public seams: authenticated fleet remediation-policy Admin API and Git Sync
  composition for fleet desired/cron artifacts.
- Red: the policy seam did not compile before the mutator dependency existed;
  no authenticated route could establish a fleet barrier.

## OS-USF-011 — selectable memory and Redis backends

- Public seams: server environment configuration, authenticated Sync, and
  authenticated fleet-policy mutation.
- Intended red: `TestFastPathBackendSelection` failed to compile because
  `FastPathConfig` had no backend selection.
- Green: `disabled`, `memory`, and `redis` are accepted; memory is the non-Fly
  default, the legacy `false` switch overrides to disabled, multi-process
  memory is rejected, and Redis requires a credential-bearing URL plus a
  bounded namespace without exposing the URL in errors or status.
- Real Redis 7.4 evidence passes for process replacement, shared fleet
  invalidation, checkpoint claim transfer/completion, and connection failure.
  An authenticated Admin mutation returns 503 with zero persistence calls when
  the Redis begin barrier is unavailable.
- The bounded reusable pool retained five authenticated connections across the
  two-process integration fixture rather than authenticating every command;
  lookup/fill/barrier/checkpoint operations executed through primary `EVAL`.

## OS-PSA-018 — Redis replacement and outage load

- Disposable Compose used Redis 7.4 with `noeviction`, Postgres 16, and the
  real server. Background agent fixtures were stopped to isolate the 400
  load identities.
- Process replacement: all 2,400 Syncs succeeded. Bounded server diagnostics
  after restart reported backend `redis`, 400 hits, zero misses, zero Redis
  errors, and zero application database operations. Replacement-hit p95 was
  179.5 ms and the following shared-hit wave was database-free.
- Redis pause/recovery: all 2,800 Syncs succeeded. The 400-request outage wave
  fell back to Postgres (p95 3.08 s); the recovery waves immediately returned
  to zero-database shared hits. No deadlocks, temp files, or rollbacks grew.
- A deliberately misconfigured first attempt used fleet `default`, which is
  absent from the Compose repository and correctly returned artifact-resolution
  errors. A second diagnostic attempt revealed that the restart subprocess had
  reverted to memory mode; the target now preserves the Redis environment and
  the final run verified the intended topology.

## Redis extension final verification

- Focused server race tests: PASS.
- Real Redis 7.4 integration for identity, namespace, expiry, entry/byte bounds,
  malformed values, replacement, shared mutation, checkpoint claim retry, and
  outage: PASS.
- Fly bootstrap execution tests and shell syntax checks: PASS.
- Performance budget lint: PASS with 32 approved metrics.
- Strict OpenSpec validation: PASS.
- `make test`: PASS, including all 67 discovered fuzz seed corpora and the
  complete root-module package suite.
- Green: `PUT /v1/admin/fleets/{fleet}/remediation-policy` validates fleet and
  policy before beginning a fleet generation barrier, then evicts only that
  fleet before persistence. The mutation callback observes the target scope as
  unstable and the unrelated fleet still cached. Both success and injected
  persistence failure leave the target invalidated. Fleet desired and cron
  composition remains under the global Git Sync barrier because one repository
  transaction may change multiple fleets atomically.
- Focused fleet-policy and global composition invalidation tests pass.

## OS-USF-006 endpoint invalidation

- Public seams: enrollment/credential registration, endpoint deletion
  (revocation), label targeting, and the authenticated endpoint reassignment
  Admin API.
- Red: registration observed its old entry still present; label mutation
  panicked at its persistence callback because eviction had not begun; the
  reassignment route returned 404.
- Green: each seam validates its request, begins an endpoint generation barrier
  before durable mutation, and completes on every outcome. A new
  `PUT /v1/admin/endpoints/{id}/fleet` seam persists reassignment in both memory
  and Postgres adapters. Public callback assertions prove revocation/targeting
  persistence runs only after eviction and while authority is unstable, while
  an unrelated endpoint remains cached. Enrollment covers both server-issued
  credential registration and the existing CSR credential path at their shared
  registration boundary.
- Focused endpoint public-seam tests and the authenticated mutation recovery
  test pass under the race detector; Postgres reassignment/delete adapter tests
  pass.

## OS-USF-006 remaining mutation classes

- Red: the centralized completeness test did not compile because mutation
  classes and their scope registry were absent.
- Green: a fail-closed registry now declares release/schedule, fleet policy and
  upgrade, endpoint enrollment/delete/reassignment/labels/upgrade/diagnostics,
  change-control, secret lifecycle, and enrollment/deployment-token scopes.
  Unknown future classes default to global invalidation. The public handlers
  begin the declared barrier after request validation and before persistence;
  schedule changes are covered by their repository release transaction.
- Deterministic table cases prime target, same-fleet, and unrelated-fleet
  decisions for every class and assert exact global/fleet/endpoint eviction.
  Existing authenticated handler suites for upgrades, diagnostics,
  change-control authorization/lifecycle, secrets, and tokens pass alongside
  the registry test; the scoped registry and endpoint public tests pass under
  the race detector.

## OS-USF-007 checkpoint configuration

- Red: command configuration tests did not compile before the checkpoint
  interval was part of `FastPathConfig`.
- Green: startup defaults to five minutes, accepts inclusive five-through-ten
  minute durations (including fractional durations), and rejects malformed,
  shorter, or longer values. The value is carried into the server cache policy
  and uses the already injected server clock for deadline evaluation.

## OS-USF-007 steady checkpoint window

- Public seam: authenticated Sync with instrumented persistence and audit
  boundaries.
- Green evidence proves a quiet cache hit adds zero registry operations, zero
  liveness writes, and zero audit events. At the injected deadline, the first
  eligible request performs exactly one additional liveness write and one
  `agent.sync.checkpoint` audit event. Its classified details contain exact
  injected window bounds and `observations=2` (one hit plus the deadline
  request), with no raw document content.
- The bounded cache-wide observation cap from OS-USF-009 also bounds aggregate
  checkpoint count retention.

## OS-USF-007 immediate security audit

- Red: an unauthenticated Sync returned 401 but produced no audit event because
  anonymous default API events were dropped.
- Green: rejected `/v1/sync` requests retain immediate bounded API audit events;
  focused unauthorized and authenticated-malformed cases record their exact
  401/400 status. Eligibility tables continue to force telemetry, results,
  intents, full documents, overload, and command-bearing work off the cache.
  Checkpoint suppression applies only after a durable aggregate event and is
  explicitly resumed for changed or one-shot response work. Administrative
  middleware remains outside the Sync bypass.
- Focused overload, fail-closed eligibility, audit, and redaction canary tests
  pass.

## OS-USF-007 checkpoint failure and restart

- Injected liveness persistence failure returns 503, retains the bounded
  checkpoint for retry, and leaves zero eligible entries. A later authenticated
  request completes the same checkpoint before full-document recovery can
  re-prime the endpoint.
- Reconstructing `Server` with the same durable registry starts with an empty
  process-local cache; the first authenticated hash request performs storage
  work and follows the full document-recovery path.
- Mutation invalidation also clears pending checkpoints at global, fleet, or
  endpoint scope so a pre-mutation checkpoint can never restore stale
  authority. All time transitions use the injected clock with no sleeps.

## OS-USF-010 bounded observability

- Red: the controlled diagnostics JSON omitted every unchanged-Sync signal.
- Green: it now exposes bounded hit, stable miss-reason, scoped invalidation,
  eviction, document request, checkpoint outcome, disabled-state, decision
  latency, response-byte, and database-operation aggregates. Eligible hits
  explicitly observe zero database operations.
- All dynamic inputs pass through fixed allowlists with an `other` bucket.
  A synthetic secret/high-cardinality canary injected as invalidation,
  document, and disabled reasons is absent from serialized diagnostics; hashes,
  endpoint IDs, certificates, raw documents, headers, and payloads are never
  metric keys or values.

## Native unchanged-Sync decision benchmark

- `BenchmarkUnchangedSyncDecision` uses a representative capability plus
  system-information hash-only request and separate hit/absent-miss fixtures.
  It reports allocations and is mapped into versioned required budgets for hit
  latency/bytes/allocations and miss latency.
- Five controlled samples: hit 1,384–1,604 ns/op, 1,058 B/op, 10 allocs/op;
  miss 107.8–108.5 ns/op, 0 B/op, 0 allocs/op. All are well within the newly
  adopted explicit budgets; the performance budget parser/gate suite passes.

## OS-PSA-004 / OS-PSA-015 authenticated 400-identity load

- The harness now carries accepted hashes per unique mTLS identity, primes full
  capability plus system-information documents, switches to hash-only, and
  selectively retransmits requested documents. Every wave has its own
  PostgreSQL delta.
- Controlled repeat, eligible hit: 400/400 successes, 400 unchanged, p95
  39.094312 ms, 143,200 request bytes, zero artifact response bytes, and exact
  zero deltas for commits, blocks, tuples, inserts, temp files, and deadlocks.
  Runtime observation independently records 400 hit samples with database
  operations total/max both zero.
- Five-minute checkpoint scenario: 400 deadlines and exactly 400 successful
  aggregate checkpoint units (one liveness plus one audit record per endpoint),
  with no errors. After document recovery the final 400-request hit phase again
  reported zero PostgreSQL counter deltas. Bounded diagnostics after the run
  showed `checkpoints={due:400,success:400}` and no failure bucket.

## OS-PSA-016 / OS-PSA-017 invalidation and recovery load

- Release fan-out/rollback, fleet policy, and endpoint revocation boundaries are
  exercised at their authenticated public seams by the deterministic generation
  tests; those tests prove eviction occurs before the mutation callback and run
  under the race detector. The load harness no longer treats a database side
  channel as equivalent to those public mutation boundaries.
- Bounded eviction run (`maxEntries=200`, 400 identities): 400/400 successful,
  zero errors, exactly 200 hits plus 200 document-recovery responses, exactly
  200 LRU evictions, zero PostgreSQL deltas in the eviction wave, p95 59.946293
  ms, and six server goroutines after the wave.
- Actual server-container cold restart retained all 400 mTLS clients. The first
  post-restart wave took the full recovery path; document recovery then produced
  400 unchanged responses, and the final 400-request hit wave had p95 36.480052
  ms and exact zero PostgreSQL counter deltas. No wave had an error, overload,
  temp-file increase, deadlock, artifact-row growth, or retained-row growth
  beyond the 400 newly provisioned endpoint identities.

## Final verification gate — 2026-08-07

- Focused race gate: the authenticated zero-storage path, fail-closed
  eligibility, resource bounds, mutation barrier, Git/Fleet/endpoint public
  invalidations, and immediate rejection audit selectors pass under `-race`.
- Full Postgres package with `-tags=postgres`, plus the agent, load-harness,
  and load-command packages, pass against the disposable Compose database.
- `FuzzDecodeSummaryBoundedRoundTrip` completed 1,876,898 executions in the
  bounded five-second campaign with no crash; all committed fuzz seed corpora
  also pass through `make test`.
- Five final benchmark samples report hit 1,893–5,508 ns/op, 1,057–1,058 B/op,
  10 allocs/op; miss 109.7–142.7 ns/op, 0 B/op, 0 allocs/op. Every sample is
  within the versioned budgets.
- `go test -mod=vendor ./internal/traceability ./internal/performance
  ./cmd/remotr-server -count=1` passes with all fourteen verification IDs
  mapped and no new evidence exception.
- Final `make test` passes: 67 discovered fuzz seed targets, release-catalog
  checks, every root-module package, acceptance suite, benchmark fixtures, and
  provider-contract support packages.

## Completion-audit correction — four-domain document contract

- The post-gate audit reopened OS-USF-003/004/005: the protocol vocabulary
  named `delivery` and `targeting`, but the production agent emitted hashes
  only for capability and system information, targeting inputs were never
  elided, and authenticated Sync acknowledged neither additional domain.
- Agent public seam red: the expanded restart wire test failed because the
  initial request had no complete targeting document/hash and later requests
  could not prove four-domain hash-only behavior. Green: the agent now derives
  canonical delivery and targeting documents, emits all current hashes,
  persists their accepted values, elides accepted targeting inputs, and
  retransmits changed or requested content. Focused lost-response, change,
  request, and restart cases pass.
- Authenticated Sync red: the expanded persistence-order test returned only the
  capability hash as accepted. Green: delivery and targeting declarations are
  independently recomputed, mismatches are rejected before persistence, and
  acceptance follows the durable delivery transition or changed-only targeting
  store. Injected targeting persistence failure returns 503 without accepting
  its hash.
- The load endpoint model now primes and compares capability, system
  information, delivery, and targeting hashes, eliding only acknowledged
  content and restoring selectively requested documents. Its focused protocol
  test and the full server/load-harness suites pass.

### Completion-audit persistence and load evidence

- Real Postgres targeting persistence passes against temporary production-shape
  tables. Reordered equal labels/usernames return `changed=false` and retain
  row `xmin`; changed content atomically replaces labels, removes stale labels,
  and clears usernames when the complete submitted set is empty.
- The first four-domain 400-identity run was intentionally rejected as evidence:
  its second wave returned 400 capability blocks. That exposed a cold/full-path
  gap where a hash matching durable capability or system-information content
  was requested again rather than restored as validated evidence.
- Focused authenticated red: `TestEligibleUnchangedSyncPerformsZeroStorageOperations`
  returned a capability block and requested `capability` for its durably matched
  hash-only prime. Green: the server now independently hashes and validates the
  durable canonical capability body and system-information report, accepts only
  exact matches, requests mismatches, and performs no semantic rewrite merely
  to reconstruct the full-path decision.
- A second load audit found that `load-steady-400` stopped after the artifact
  wave and decision-prime wave, before measuring an actual cache hit. The new
  focused harness test failed with two requests instead of the required three.
  Green: steady load now separates `artifact-warm-up`,
  `unchanged-decision-prime`, and `steady-unchanged-1`.
- Corrected controlled run: the hit wave completed 400/400 successful and
  unchanged, zero blocks/errors, p95 47.566786 ms, 211,600 request bytes, zero
  artifact bytes, and zero blocks read/hit, tuples returned/fetched/inserted,
  retained rows, temp files, and deadlocks. Bounded server metrics independently
  recorded exactly 400 hits and `databaseOperations={count:400,total:0,max:0}`.

### Completion-audit verification gate

- Focused four-domain agent, authenticated Sync, load-harness, registry,
  document-hash, and Postgres package suites pass. The real Postgres package
  also passes with `-tags=postgres`.
- Focused authenticated hash validation, durable-before-ack, durable hash-only
  restoration, and zero-storage tests pass under the race detector.
- `FuzzDecodeSummaryBoundedRoundTrip` completed 1,549,340 executions in the
  bounded five-second campaign with no crash.
- Five native benchmark samples report hit 1,580–5,503 ns/op, 1,057–1,058 B/op,
  10 allocs/op; miss 107.7–110.8 ns/op, 0 B/op, 0 allocs/op. All remain within
  the versioned budgets.
- Traceability, performance-budget, and `remotr-server` configuration suites
  pass with the new delivery/targeting selectors.
- Final `make test` passes after the correction: all 67 fuzz seed targets,
  release-catalog checks, root-module packages, acceptance tests, benchmark
  fixtures, and provider-contract support packages are green. The disposable
  Compose stack and volumes were removed after verification.
