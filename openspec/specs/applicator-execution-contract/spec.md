# Applicator Execution Contract Specification

## Purpose

Define exact, evidence-backed applicator qualification and release gates for supported endpoint platforms.

## Requirements

### Requirement: Ubuntu support claims are exact and evidence-backed
An Ubuntu applicator capability SHALL be advertised only for an exact capability ID, provider/backend, contract revision, distribution, release, architecture, and required evidence environment whose complete selectors pass. For this qualification change the platform tuple SHALL be Ubuntu 24.04 amd64. Broad family discovery or a passing sibling resource SHALL NOT imply support for another resource, field, backend, release, architecture, or risk behavior.

#### Scenario: Ubuntu capability lacks an exact passing row
<!-- verification-id: OS-AEC-093 -->
- **WHEN** an agent can construct a provider on Ubuntu 24.04 but its exact contract row is missing, untested, planned, skipped, failing, or incomplete
- **THEN** the capability remains unadvertised and release validation does not treat implementation presence as support

#### Scenario: Broad family row covers only one contract
<!-- verification-id: OS-AEC-094 -->
- **WHEN** one identity, service, filesystem, security, network, desktop, or other family contract passes but sibling contracts lack their own required evidence
- **THEN** only the exact passing contract may be advertised and the family row cannot authorize the siblings

### Requirement: Ubuntu qualification includes public composition proof
A source-only, checked-in schema-1 configuration repository SHALL contain representative resources for every non-package contract intended for Ubuntu 24.04 qualification. The public configuration CLI SHALL validate, discover, and deterministically render that repository while preserving every expected stable resource address, accepted managed field, dependency, provider requirement, ownership, policy, and activation intent. High-risk examples SHALL remain non-enforcing without explicit isolated-fixture authorization. Generated `desired.yaml` and `crons.yaml` artifacts SHALL NOT be committed to the source repository.

#### Scenario: Qualification repository renders
<!-- verification-id: OS-AEC-095 -->
- **WHEN** the Ubuntu 24.04 qualification repository is validated, discovered, and rendered for its declared fleet through the public operator CLI
- **THEN** semantic assertions find every expected representative resource and field in deterministic canonical output without applying endpoint state

#### Scenario: Registered contract is omitted from qualification inventory
<!-- verification-id: OS-AEC-096 -->
- **WHEN** a non-package Ubuntu-targetable resource/provider contract is registered without a composed fixture and an explicit `qualified`, `blocked`, or `unadvertised` disposition
- **THEN** qualification validation fails rather than silently excluding the contract from the support audit

### Requirement: Qualification evidence environment follows behavior risk
Every Ubuntu 24.04 qualification row SHALL pass the common provider contract and the additional real-environment evidence selected by its behavior. Container evidence MAY qualify ordinary non-service POSIX behavior. Access, connectivity, boot, storage, firewall, authentication, system-service, kernel-security, desktop/session, and other destructive-safety behavior SHALL use the applicable Ubuntu 24.04 VM safety/recovery fixture. Secret-bearing behavior SHALL include secret-canary/redaction and cleanup evidence. Containers SHALL NOT substitute for required VM recovery evidence.

#### Scenario: Ordinary POSIX provider passes in Ubuntu container
<!-- verification-id: OS-AEC-097 -->
- **WHEN** an ordinary non-service POSIX contract passes compliant, drifted, Apply, second Check, absence, unsupported, failure, lock, cancellation, activation, redaction, rollback, and preservation cases applicable to it in the pinned Ubuntu 24.04 container
- **THEN** its container evidence condition may be satisfied for the exact contract row

#### Scenario: High-risk provider has container evidence only
<!-- verification-id: OS-AEC-098 -->
- **WHEN** an access, connectivity, boot, storage, firewall, authentication, kernel-security, desktop/session, or other destructive-safety contract passes unit and container tests but lacks its required Ubuntu 24.04 VM verification and recovery selector
- **THEN** the exact contract row remains unadvertised

#### Scenario: Secret-bearing provider leaks a canary
<!-- verification-id: OS-AEC-099 -->
- **WHEN** a qualification run finds secret-canary material in desired-state output, argv, logs, diagnostics, rollback state, Sync, persistence, API, or CLI output outside its authorized sink
- **THEN** qualification fails and no affected capability row is advertised

### Requirement: Qualification inventory remains truthful through correction and closeout
The qualification inventory SHALL be checked against registered Ubuntu-targetable contracts and SHALL record each exact row as `qualified`, `blocked`, or `unadvertised` with its governing verification IDs, composed fixture address, evidence class, selectors, and reason. A provider defect found during qualification SHALL be corrected through a focused red-green behavioral slice at an approved public seam or SHALL keep the row blocked. The refreshed M1–M5 audit SHALL derive milestone and archive decisions from composition, matrix, traceability, safety/recovery, capability-advertisement, and dependent-change status; planned or missing evidence SHALL NOT be reported as complete.

#### Scenario: Real provider contradicts focused tests
<!-- verification-id: OS-AEC-100 -->
- **WHEN** a real Ubuntu 24.04 qualification fixture exposes behavior that contradicts the accepted contract or command-boundary tests
- **THEN** a focused public-seam test records the red behavior and the row remains blocked until the minimum implementation and required broader evidence pass

#### Scenario: One milestone retains a blocked row
<!-- verification-id: OS-AEC-101 -->
- **WHEN** the refreshed M1–M5 audit finds a non-optional contract with a blocked, planned, missing, skipped, failing, or untested requirement
- **THEN** that milestone and the umbrella archive gate remain incomplete with the exact blocking row and selector reported

#### Scenario: Provider evidence passes before shared gates
<!-- verification-id: OS-AEC-102 -->
- **WHEN** a provider row passes but its required execution-contract, capability-delivery, package-provider, or testing-foundation dependency is not accepted
- **THEN** the evidence remains recorded but capability advertisement and final umbrella qualification stay blocked

#### Scenario: Ubuntu qualification closes
<!-- verification-id: OS-AEC-103 -->
- **WHEN** every non-optional Ubuntu 24.04 target is qualified or explicitly descoped by an approved specification update and all shared dependencies, public composition, provider, traceability, safety, and release audits pass
- **THEN** the audit may report the exact supported capabilities as qualified and allow the umbrella archive gate to proceed without implying future-roadmap or compliance coverage
