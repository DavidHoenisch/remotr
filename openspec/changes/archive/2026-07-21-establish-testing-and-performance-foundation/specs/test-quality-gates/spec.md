## ADDED Requirements

### Requirement: Implementation follows vertical-slice TDD
Each implementation slice SHALL name its verification IDs and public seams, demonstrate a test failing for the intended missing behavior before production implementation, and add only enough behavior to make the focused slice pass before continuing.

#### Scenario: AI agent begins an implementation task
<!-- verification-id: OS-TQG-001 -->
- **WHEN** the agent has not identified the governing verification IDs, test seam, and red command
- **THEN** the task remains unstarted and production code is not changed

#### Scenario: New test fails for an unrelated setup error
<!-- verification-id: OS-TQG-002 -->
- **WHEN** the initial red result is caused by broken fixtures, compilation unrelated to the intended interface, or unavailable infrastructure
- **THEN** it is not accepted as red evidence for the behavioral slice

### Requirement: Tests observe agreed public seams
Tests SHALL verify behavior through configuration authoring, operator API/CLI, Sync protocol, agent execution, provider contract, system-safety, or performance seams. Tests SHALL NOT couple to private algorithms or use persistence inspection as a shortcut around a public read interface.

#### Scenario: Internal implementation is refactored
<!-- verification-id: OS-TQG-003 -->
- **WHEN** observable behavior is unchanged but private collaborators or call order change
- **THEN** behavior-focused tests continue to pass without rewritten expectations

#### Scenario: Exact process invocation is security behavior
<!-- verification-id: OS-TQG-004 -->
- **WHEN** a provider must prove argv separation, noninteractive flags, or absence of unsafe shell evaluation
- **THEN** a test may assert exact external-process arguments at that boundary

### Requirement: Test doubles stop at controlled external boundaries
Mocks and fakes SHALL represent operating-system commands, time, randomness, network peers, external services, or replaceable persistence boundaries. Owned internal Remotr modules SHALL normally be composed through their public interfaces rather than mocked.

#### Scenario: Domain service calls another owned module
<!-- verification-id: OS-TQG-005 -->
- **WHEN** both modules can run deterministically in-process
- **THEN** the test composes their real implementations instead of asserting mock call counts

### Requirement: Pull requests run the fast deterministic quality gate
Every pull request SHALL run formatting/vetting, strict OpenSpec validation, traceability lint, ordinary Go tests with coverage, race detection, fuzz seed corpora, Godog in-process acceptance, and affected provider contract tests. The required gate SHALL target completion within ten minutes through parallelism and focused selection.

#### Scenario: Pull request changes server Sync behavior
<!-- verification-id: OS-TQG-006 -->
- **WHEN** the pull request is evaluated
- **THEN** ordinary tests, race tests, relevant acceptance scenarios, traceability, and affected contract tests are required before merge

### Requirement: Expensive assurance has required scheduled and release cadence
The repository SHALL run active fuzzing, the full container matrix, VM safety tests, fleet load tests, soak tests, and mutation campaigns at documented nightly, weekly, or release cadences. A required expensive gate MAY run in a protected merge queue but SHALL NOT become advisory solely because of runtime.

#### Scenario: Network provider change needs a VM test
<!-- verification-id: OS-TQG-007 -->
- **WHEN** the fast PR tests pass but required network recovery VM evidence has not completed
- **THEN** protected merge or release remains blocked until that evidence passes

### Requirement: Coverage is risk-based and ratcheted
Coverage SHALL be reported by package and changed code, SHALL explicitly exclude generated sources, and SHALL prevent unexplained regression from an approved baseline. Critical decision modules SHALL receive documented floors after their first complete slice; no single repository-wide percentage SHALL substitute for behavioral evidence.

#### Scenario: New authorization branch is untested
<!-- verification-id: OS-TQG-008 -->
- **WHEN** changed-line coverage shows the branch is not executed
- **THEN** the quality gate fails even if repository-wide coverage increased elsewhere

### Requirement: Race detection is continuous
The Go race detector SHALL run on the deterministic repository suite for every pull request and on realistic server/agent workloads at scheduled cadence.

#### Scenario: Concurrent telemetry change races
<!-- verification-id: OS-TQG-009 -->
- **WHEN** exercised code performs unsynchronized conflicting access
- **THEN** the race job fails and the change cannot merge on a normal retry

### Requirement: Fuzz target discovery cannot silently omit tests
Fuzz orchestration SHALL discover all native Go fuzz targets, verify that every selected expression matches a target, and fail on missing, omitted, or zero-match targets. Seed corpora SHALL run during ordinary Go tests.

#### Scenario: Fuzz function is renamed
<!-- verification-id: OS-TQG-010 -->
- **WHEN** the source target name changes but orchestration is stale
- **THEN** validation fails instead of accepting a warning with no fuzz execution

### Requirement: Fuzz failures become permanent regression evidence
A native fuzz target SHALL assert durable safety or correctness properties. Any minimized failing input SHALL be committed to its corpus with a regression reference before the defect is considered resolved.

#### Scenario: Parser fuzzing finds a crash
<!-- verification-id: OS-TQG-011 -->
- **WHEN** Go fuzzing minimizes an input that causes the crash
- **THEN** the minimized corpus entry runs in ordinary tests and remains after the repair

### Requirement: Mutation testing gates new critical logic
A pinned mutation tool SHALL be validated against Remotr before adoption. Once adopted, new critical selection, authorization, rollback, secret, ordering, and validation logic SHALL have no unexplained surviving relevant mutants. Equivalent or intentionally accepted survivors SHALL have reviewed durable dispositions.

#### Scenario: Conditional mutation survives new tests
<!-- verification-id: OS-TQG-012 -->
- **WHEN** mutation testing reverses an authorization boundary and the focused suite still passes
- **THEN** the implementation slice is incomplete until a killing test or reviewed equivalent-mutant disposition exists

### Requirement: Godog runs as ordinary Go test infrastructure
Godog SHALL be pinned and vendored, isolated behind a dedicated acceptance package, runnable through `go test`, and free of production dependency wiring.

#### Scenario: Godog library changes incompatibly
<!-- verification-id: OS-TQG-013 -->
- **WHEN** the pinned dependency is intentionally upgraded
- **THEN** compatibility changes are confined to the acceptance wrapper and feature semantics remain unchanged

### Requirement: Flaky and skipped tests remain visible defects
Required tests SHALL NOT use silent retry or permanent skip to manufacture success. Temporary quarantine SHALL record an owner, tracked defect, equivalent safety signal, and expiry.

#### Scenario: Required VM scenario becomes intermittent
<!-- verification-id: OS-TQG-014 -->
- **WHEN** the scenario fails nondeterministically
- **THEN** it remains release-visible and can be quarantined only with the required ownership and expiry metadata

### Requirement: Completion evidence is reviewable
An implementation task SHALL NOT be marked complete until its traceability entries are verified and its required unit, contract, acceptance, provider, fuzz, mutation, safety, and performance evidence passes according to risk.

#### Scenario: Provider unit tests pass without real environment evidence
<!-- verification-id: OS-TQG-015 -->
- **WHEN** a task would advertise the provider but its required matrix entry has not passed
- **THEN** the task remains incomplete and the capability stays unadvertised
