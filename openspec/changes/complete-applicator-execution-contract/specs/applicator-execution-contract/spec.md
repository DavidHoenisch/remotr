## MODIFIED Requirements

### Requirement: Rollback storage is centralized and bounded
Rollback metadata and payloads SHALL be stored only in a root-owned agent transaction store keyed by Resource address, artifact digest, and attempt; applicators SHALL NOT leave generic adjacent backup files. A rollback-advertising provider SHALL reserve sufficient durable capacity and arm its recovery record before mutation. The store SHALL retain at most 10 attempts per Resource and 30 days of metadata, at most three successful non-secret prior states per Resource and 30 days, and sensitive or secret payloads only while armed or unacknowledged with an absolute 24-hour maximum. A configurable global disk cap SHALL also apply, but an armed recovery SHALL NOT be pruned to satisfy it. Startup SHALL validate active records and SHALL block affected rollback-requiring mutation when an armed record is corrupt, unavailable, or cannot be decrypted.

#### Scenario: Adjacent managed file is replaced
<!-- verification-id: OS-AEC-068 -->
- **WHEN** a file provider stages rollback for `/etc/example.conf`
- **THEN** it stores protected rollback under agent state and does not create `/etc/example.conf.remotr.bak`

#### Scenario: Disk cap would remove armed rollback
<!-- verification-id: OS-AEC-069 -->
- **WHEN** available rollback capacity cannot retain an armed payload
- **THEN** Apply is blocked before mutation rather than pruning the armed recovery state

#### Scenario: Agent restarts with an armed transaction
<!-- verification-id: OS-AEC-080 -->
- **WHEN** the agent process restarts after rollback is armed but before acknowledgement
- **THEN** startup validates and restores the same transaction by resource address, artifact digest, and attempt before that resource can mutate again

#### Scenario: Per-resource retention limit is exceeded
<!-- verification-id: OS-AEC-081 -->
- **WHEN** cleanup evaluates more than 10 terminal attempts or more than three successful non-secret prior states for one Resource
- **THEN** it deterministically prunes only eligible oldest records while preserving every armed or otherwise retained recovery

### Requirement: Durable rollback payloads are encrypted
Durable rollback payloads SHALL use authenticated encryption under a versioned endpoint-local rollback key identity. TPM sealing SHALL be used only when an advertised supported TPM provider is available; a root-only key file MAY be used with explicit reduced-protection reporting that it does not protect against endpoint-root compromise. Payload and metadata SHALL form one checksummed, crash-safe, atomically activated record. Key rotation SHALL retain decrypt-only access for every referenced armed or retained record.

#### Scenario: TPM is unavailable
<!-- verification-id: OS-AEC-070 -->
- **WHEN** the endpoint has no supported TPM sealing provider
- **THEN** rollback may use a root-only local key while capability/reporting states the reduced protection

#### Scenario: TPM provider fails after being selected
<!-- verification-id: OS-AEC-082 -->
- **WHEN** policy selected TPM protection but the provider cannot seal or load the required key
- **THEN** new rollback-requiring Apply is blocked and the agent does not silently downgrade that transaction to a root-file key

### Requirement: Rollback cleanup follows terminal state
Rollback payloads SHALL be cleaned after successful acknowledgement, completed rollback, expiry, supersession, or explicitly authorized abandonment according to sensitivity and retention policy. Cleanup SHALL be deterministic, restart-safe, and unable to remove an armed record merely to admit new work. The server SHALL store only safe rollback metadata and outcomes.

#### Scenario: Transaction acknowledges successfully
<!-- verification-id: OS-AEC-073 -->
- **WHEN** a sensitive transactional change receives final acknowledgement
- **THEN** its local secret-bearing payload is destroyed promptly while non-secret audit metadata remains within retention bounds

### Requirement: Typed redaction
The schema SHALL classify every accepted field as public, sensitive metadata, or secret. Registration or validation SHALL fail when an accepted field lacks a classification. Secret values SHALL be projected into approved safe metadata or omitted before entering logs, reports, diffs, backups, diagnostics, rollback metadata, or persistent telemetry; generic sinks SHALL accept only classified safe values rather than arbitrary desired or observed resource objects.

#### Scenario: Secret-backed resource drifts
<!-- verification-id: OS-AEC-074 -->
- **WHEN** Check evaluates a secret-backed resource
- **THEN** the report may include its reference, safe fingerprint, and health but never the secret value

#### Scenario: Accepted field lacks sensitivity metadata
<!-- verification-id: OS-AEC-083 -->
- **WHEN** a resource schema registers an accepted field without a public, sensitive-metadata, or secret classification
- **THEN** registration and repository validation fail before the field can reach Check or Apply

#### Scenario: Generic backup receives a secret-bearing resource
<!-- verification-id: OS-AEC-084 -->
- **WHEN** backup or diagnostic collection attempts to serialize desired or observed state containing a secret field
- **THEN** the sink receives only its approved safe projection and a secret canary is absent from the produced artifact

## ADDED Requirements

### Requirement: Effective desired-state hashes are canonical and secret-safe
The system SHALL compute each Resource's effective desired-state hash from one versioned canonical representation after schema normalization, defaults, provider selection, and secret-reference resolution to safe provider/version identity. The representation SHALL distinguish omitted unmanaged fields from explicit values, sort unordered structures, include provider contract revision, and exclude runtime observations, timestamps, endpoint identity, randomness, and secret bytes. Change requests, baselines, leases, agents, and reports SHALL reject a supplied hash that does not match recomputation at their trusted boundary.

#### Scenario: Active secret version changes
<!-- verification-id: OS-AEC-085 -->
- **WHEN** a Resource follows an active secret reference and its provider activates a new version with identical secret bytes
- **THEN** the effective hash changes using safe version identity, contains no secret bytes, and no authorization bound to the prior hash remains eligible

### Requirement: High-risk plans are derived from composed state
The server SHALL derive non-enforcing high-risk plans from composed registered Resources, canonical effective hashes, provider contract revisions, risks, authorization groups, dependencies, activation targets, typed predicted effects, rollback classes, and baseline eligibility. Admin API clients SHALL NOT supply authoritative Resource hashes or effects. Current authenticated endpoint capability and non-enforcing Check/preflight evidence SHALL be joined before target freeze. Dependency failure, current preflight failure, rollback-reservation failure, redaction failure, or hash mismatch SHALL block affected high-risk work and SHALL NOT be bypassed by break glass.

#### Scenario: Client submits a different desired hash
<!-- verification-id: OS-AEC-086 -->
- **WHEN** an Admin API request attempts to create authorization using a Resource hash different from the server's composed canonical hash
- **THEN** the request is rejected rather than storing or authorizing the caller-supplied plan

#### Scenario: Normal dependency cannot reserve rollback
<!-- verification-id: OS-AEC-087 -->
- **WHEN** a high-risk Resource depends on a normal Resource whose required transactional mutation cannot reserve recovery capacity
- **THEN** the derived plan records the dependency block and neither ordinary authorization nor break glass permits the dependent high-risk Apply
