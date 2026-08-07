## ADDED Requirements

### Requirement: Eligible unchanged Sync is database-free
After authenticated endpoint identity and bounded request validation, the server SHALL answer an eligible unchanged Sync entirely from a bounded in-memory decision and SHALL perform zero database queries, commands, transactions, or audit writes for that request. The response SHALL preserve the exact release, artifact digest, remediation policy, crons digest, and other stable public fields established by the last full Sync.

#### Scenario: Quiet endpoint repeats an accepted state
<!-- verification-id: OS-USF-001 -->
- **WHEN** an authenticated endpoint repeats the accepted release, artifact digest, and document hashes before any checkpoint or revalidation deadline and has no pending telemetry or intent
- **THEN** Sync returns the unchanged response with no artifact bytes and instrumented persistence boundaries observe zero database operations

### Requirement: Fast-path eligibility fails closed
The server SHALL use the in-memory path only when authenticated identity, endpoint authority, release acknowledgement, all response-affecting document hashes, authority generations, checkpoint time, and time-dependent work are known and unchanged. Full documents, telemetry, results, acknowledgements, intents, unknown fields that affect selection, capability blocks, due schedules, diagnostics, upgrades, execution leases, and other one-shot work SHALL bypass the fast path. Uncertainty SHALL fall back to full Sync rather than return a cached decision.

#### Scenario: Endpoint has telemetry to persist
<!-- verification-id: OS-USF-002 -->
- **WHEN** an otherwise eligible request includes a drift transition, apply failure, cron result, diagnostic result, firewall audit change, change preflight, reboot intent, or network intent
- **THEN** the server bypasses the cache and processes that input through the authenticated full Sync path

#### Scenario: Time-dependent work becomes due
- **WHEN** the earliest cron, diagnostic, upgrade, execution, checkpoint, or other revalidation deadline is reached without an authority mutation
- **THEN** the next Sync bypasses the cached decision and reevaluates the due work

### Requirement: Document hashes are canonical, acknowledged, and bounded
Repeatable Sync documents SHALL have a versioned, document-type-domain-separated SHA-256 hash over canonical semantic bytes. The server SHALL enforce bounded names, counts, hash syntax, and document sizes; SHALL recompute the hash of every supplied full document; and SHALL reject a declared hash that does not match. A client-supplied hash SHALL NOT by itself establish capabilities, authorization, inventory, targeting, or delivery state.

#### Scenario: Full capability document has a false hash
<!-- verification-id: OS-USF-003 -->
- **WHEN** an authenticated request supplies a capability document and a different declared hash
- **THEN** Sync rejects the malformed document evidence without persisting it or priming a cache entry

#### Scenario: Server does not possess an advertised document
- **WHEN** an agent supplies only a well-formed document hash that neither memory nor durable state can associate with validated content for that endpoint
- **THEN** the response requests the named full document and the server does not use that hash to establish fast-path eligibility

### Requirement: Agents elide documents only after server acknowledgement
Agents SHALL send current hashes for repeatable capability, system-information, delivery, and targeting documents on Sync. An agent SHALL send a full document when its semantic hash changes, when the server requests it, or until the server acknowledges that hash; it SHALL persist accepted hashes across restart and SHALL NOT suppress event telemetry. Lost responses SHALL cause safe retransmission rather than permanent document loss.

#### Scenario: Accepted capability and inventory remain unchanged
<!-- verification-id: OS-USF-004 -->
- **WHEN** the server has acknowledged the current capability and system-information hashes and their semantic content has not changed
- **THEN** subsequent requests carry the hashes without the full documents until the server requests content again

#### Scenario: Successful upload response is lost
- **WHEN** an agent uploads a changed document but does not receive the server's acceptance response
- **THEN** it retains and retransmits the full document on a later Sync

### Requirement: Semantic state is written only when changed
The server SHALL persist capability documents, delivery state, system information, and other hash-elided documents only when their canonical semantic content changes. Receive, offer, active, report, or update timestamps SHALL NOT turn an otherwise equal document or delivery state into a semantic change. The server SHALL acknowledge a new hash only after required durable persistence succeeds.

#### Scenario: Full path receives equal semantic state
<!-- verification-id: OS-USF-005 -->
- **WHEN** a cache miss runs full Sync with capability, delivery, and system-information content equal to the last durable values
- **THEN** persistence reports no semantic change and issues no update for those values

#### Scenario: Delivery acknowledgement advances
- **WHEN** the endpoint acknowledges a newly offered release and digest
- **THEN** the active delivery state is durably changed before the server acknowledges the new delivery hash

### Requirement: Authoritative mutations invalidate before stale reuse
Every response- or authorization-affecting mutation SHALL invalidate affected global, fleet, or endpoint decisions no later than its durable linearization point and SHALL prevent a decision computed from unstable authority from being cached. This SHALL cover release advance or rollback, remediation policy, artifact and cron targeting, endpoint enrollment/register/delete/revoke/reassign, labels, credentials, upgrades, diagnostics, change-control authorization or leases, and all revocations at the narrowest provably safe scope. A failed mutation MAY leave entries evicted.

#### Scenario: Fleet policy changes
<!-- verification-id: OS-USF-006 -->
- **WHEN** an operator changes a fleet's remediation policy after an endpoint has entered the fast path
- **THEN** no Sync linearized after the mutation can receive the old policy from that cache entry

#### Scenario: Release advances
- **WHEN** Git Sync durably advances the release reference
- **THEN** affected endpoints miss their prior entries and reevaluate artifact selection on their next Sync

#### Scenario: Endpoint is revoked concurrently
- **WHEN** endpoint revocation races with an otherwise eligible cached Sync
- **THEN** no request linearized after the revocation invalidation point succeeds from the revoked endpoint's cache entry

#### Scenario: Mutation fails after invalidation
- **WHEN** an authority mutation invalidates its scope but durable persistence fails
- **THEN** the mutation reports failure and subsequent Sync uses the full path until a new valid decision is established

### Requirement: Liveness and successful audit records use bounded checkpoints
Eligible unchanged Sync observations SHALL remain in memory between durability checkpoints. The server SHALL default to a five-minute checkpoint interval, SHALL accept only configured intervals from five through ten minutes, and SHALL persist `last_seen` plus one bounded aggregate successful-Sync audit checkpoint on the first Sync at or after the deadline. The aggregate SHALL preserve endpoint identity, window bounds, outcome, and observation count without raw documents or secrets. Security failures, malformed requests, overload decisions, state changes, commands, and administrative mutation audits SHALL remain immediate and SHALL NOT be coalesced.

#### Scenario: Endpoint polls throughout one checkpoint window
<!-- verification-id: OS-USF-007 -->
- **WHEN** an eligible endpoint completes multiple unchanged Syncs before its five-minute default deadline
- **THEN** those requests produce no database work and the first Sync at or after the deadline persists one liveness update and one bounded aggregate audit checkpoint

#### Scenario: Server crashes between checkpoints
- **WHEN** the server process exits without a completed checkpoint
- **THEN** durable state may omit only the observations since the last completed checkpoint and a restarted server treats its cache as cold

#### Scenario: Unauthorized request occurs during a quiet window
- **WHEN** authentication or revocation rejects a Sync before the next checkpoint
- **THEN** the security-relevant audit event follows the immediate audit path rather than the unchanged-Sync accumulator

### Requirement: Cache loss and legacy protocol preserve correctness
The in-memory cache SHALL be disposable. Cold start, eviction, expiry, unsupported hash protocol, absent accepted hashes, or disabled fast-path configuration SHALL use the existing authenticated full Sync behavior. Legacy agents SHALL remain compatible and SHALL NOT be treated as hash-eligible merely because their release and artifact digest match.

#### Scenario: Server restarts with durable endpoint state
<!-- verification-id: OS-USF-008 -->
- **WHEN** an endpoint Syncs after server restart
- **THEN** the server reconstructs a valid decision through the full path before any later request can hit memory

#### Scenario: Legacy agent sends no document hashes
- **WHEN** an authenticated legacy request uses the prior Sync schema
- **THEN** it receives the compatible full-path response without a protocol error or cache eligibility

### Requirement: Cache resources and serving topology are bounded
The server SHALL bound entry count, memory bytes, entry lifetime, and retained observation data and SHALL expose deterministic eviction behavior. It SHALL disable or reject process-local fast-path operation in a multi-serving-process topology unless a synchronous invalidation coordinator provides equivalent mutation-barrier semantics.

#### Scenario: Entry budget is exhausted
<!-- verification-id: OS-USF-009 -->
- **WHEN** admitting another decision would exceed the configured entry or byte budget
- **THEN** deterministic bounded eviction occurs without discarding durable endpoint state or failing Sync correctness

#### Scenario: Multiple uncoordinated server processes are configured
- **WHEN** the server cannot prove synchronous invalidation across serving processes
- **THEN** the in-memory fast path remains disabled and Sync continues through the durable full path

### Requirement: Fast-path behavior is observable without sensitive cardinality
The server SHALL report bounded hit, miss-reason, eviction, invalidation-scope, document-request/change, checkpoint, and disabled-state metrics plus decision latency, response bytes, and database operations. Metrics and diagnostic logs SHALL NOT expose raw documents, secret values, certificate material, endpoint-scoped hash labels, or other unbounded identifiers.

#### Scenario: Eligible request misses after invalidation
<!-- verification-id: OS-USF-010 -->
- **WHEN** an authority mutation evicts an otherwise eligible decision
- **THEN** bounded metrics distinguish the invalidation and subsequent miss without labeling the endpoint or document hash

