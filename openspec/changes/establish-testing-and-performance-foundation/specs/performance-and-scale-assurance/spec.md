## ADDED Requirements

### Requirement: Critical local paths have native Go benchmarks
The repository SHALL benchmark configuration parsing/validation/composition, capability and artifact selection, dependency ordering, Check/report construction, redaction, serialization/compression, secret envelope operations, rollback policy, and critical persistence queries with representative size fixtures and allocation reporting.

#### Scenario: Artifact composition becomes nonlinear
<!-- verification-id: OS-PSA-001 -->
- **WHEN** benchmark fixtures increase from 100 to 1,000 resources
- **THEN** results expose time and allocation scaling so an unapproved nonlinear regression cannot remain hidden in a small fixture

### Requirement: Benchmark comparisons are statistically meaningful
Performance comparisons SHALL use repeated samples and statistical analysis on equivalent controlled environments. Shared-runner latency SHALL be advisory unless stability is demonstrated; deterministic allocation, payload, and size bounds MAY be hard gates there.

#### Scenario: One benchmark sample is slower
<!-- verification-id: OS-PSA-002 -->
- **WHEN** a single noisy shared-runner result exceeds the prior median
- **THEN** the repository does not declare a regression without the required repeated comparison

### Requirement: Performance budgets are versioned decisions
Each critical benchmark and load scenario SHALL receive an approved absolute or relative budget after baseline measurement. Changing a release-blocking budget SHALL require review and an OpenSpec update rather than silently replacing stored results.

#### Scenario: Hot-path change exceeds its approved budget
<!-- verification-id: OS-PSA-003 -->
- **WHEN** controlled comparison finds a significant regression beyond the allowed bound
- **THEN** the change is blocked unless the regression and revised budget are explicitly approved

### Requirement: Fleet load uses distinct authenticated endpoint identities
The load harness SHALL exercise the real server and Postgres protocol with unique authenticated endpoint identities, realistic fleet membership, capability documents, digests, and telemetry. It SHALL NOT approximate endpoint load with unauthenticated health requests.

#### Scenario: Four hundred endpoints perform unchanged Sync
<!-- verification-id: OS-PSA-004 -->
- **WHEN** the reference workload runs at the default polling interval
- **THEN** the harness records successful request rate and latency plus server, database, network, and error metrics for the real unchanged path

### Requirement: Reference load covers synchronized and heavy paths
The 400-endpoint reference suite SHALL separately test steady unchanged polling, simultaneous startup/reconnect, release fan-out, telemetry-heavy Sync, mixed schema/capability populations, endpoint overrides, and server/database degradation and recovery.

#### Scenario: Release advances for the whole fleet
<!-- verification-id: OS-PSA-005 -->
- **WHEN** 400 endpoints request a new artifact within the stress window
- **THEN** the suite measures full payload delivery, persistence pressure, latency distribution, errors, and recovery to unchanged operation

#### Scenario: Database slows during a retry burst
<!-- verification-id: OS-PSA-006 -->
- **WHEN** persistence latency rises while endpoints retry
- **THEN** the suite detects pool exhaustion, retry amplification, timeout, or recovery regressions against the approved budget

### Requirement: Future-scale comparison detects nonlinear growth
A scheduled 4,000-endpoint workload SHALL compare scaling characteristics and headroom without being represented as an advertised support promise unless separately approved.

#### Scenario: Reference workload passes but comparison collapses
<!-- verification-id: OS-PSA-007 -->
- **WHEN** 400 endpoints meet their SLO but the 4,000-endpoint comparison shows nonlinear resource or latency growth
- **THEN** the result creates visible capacity work without falsely failing the current supported-scale contract

### Requirement: Agent overhead is measured independently
Agent benchmarks SHALL measure complete parse, resolve, Check, report, and applicable Apply cycles plus idle behavior using representative artifacts. Results SHALL include wall and CPU time, peak RSS, allocations, goroutines, network bytes, disk I/O, and rollback storage use where relevant.

#### Scenario: Large compliant artifact is checked
<!-- verification-id: OS-PSA-008 -->
- **WHEN** the agent processes a representative high-resource artifact with no drift
- **THEN** the suite records the complete steady-state cycle cost rather than only parser time

### Requirement: Soak tests detect retained growth
Scheduled soak tests SHALL detect monotonic memory, goroutine, connection, database-row, queue, temporary-file, and rollback-storage growth across repeated Sync and agent cycles.

#### Scenario: Repeated unchanged cycles leak goroutines
<!-- verification-id: OS-PSA-009 -->
- **WHEN** the soak workload completes its observation window
- **THEN** the growth metric exceeds its approved bound and the test fails with a diagnostic profile

### Requirement: Performance failures capture profiles safely
Controlled benchmark, load, and soak jobs SHALL capture bounded CPU, heap, goroutine, trace, query, and system metrics sufficient for diagnosis while redacting credentials and secret-bearing payloads.

#### Scenario: Load latency exceeds p95 budget
<!-- verification-id: OS-PSA-010 -->
- **WHEN** the controlled test fails its latency gate
- **THEN** the job retains redacted profiles and workload metadata that identify the expensive path and environment

### Requirement: Sync polling is jittered within a staleness bound
Agents SHALL apply bounded startup delay and stable per-endpoint polling jitter so coordinated installation does not preserve synchronized requests. Jitter SHALL maintain a documented maximum interval between successful Sync attempts.

#### Scenario: Four hundred agents start together
<!-- verification-id: OS-PSA-011 -->
- **WHEN** all agents become ready at the same instant
- **THEN** their initial and subsequent request times spread across the configured window without exceeding the maximum staleness bound

### Requirement: Transient retry uses capped jittered backoff
Transient network, overload, and server failures SHALL use capped exponential backoff with jitter and SHALL reset after a successful Sync. Permanent credential, enrollment, and validation failures SHALL follow distinct bounded policies.

#### Scenario: Server is temporarily unavailable
<!-- verification-id: OS-PSA-012 -->
- **WHEN** endpoints receive transient failures for several attempts
- **THEN** aggregate request volume decays within the configured cap and normal polling resumes after recovery without synchronized retry waves

### Requirement: Authenticated overload signaling is honored
The server MAY return a bounded `Retry-After` overload response on authenticated Sync, and the agent SHALL honor it without reporting convergence or discarding pending telemetry.

#### Scenario: Server asks an endpoint to retry later
<!-- verification-id: OS-PSA-013 -->
- **WHEN** an authenticated Sync receives the overload response
- **THEN** the agent retains pending state, schedules within the allowed retry bound, and does not treat the attempt as successful

### Requirement: Clock and randomness are injectable for deterministic tests
Polling, backoff, authorization windows, leases, expiry, and performance scheduling SHALL use injectable clock and randomness boundaries in deterministic tests.

#### Scenario: Backoff distribution is tested
<!-- verification-id: OS-PSA-014 -->
- **WHEN** a seeded randomness source and fake clock drive repeated failures
- **THEN** the test proves exact bounds and reset behavior without sleeping on wall-clock time
