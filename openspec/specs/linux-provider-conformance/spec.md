# Linux Provider Conformance Specification

## Purpose

Define reusable provider contracts and real-environment evidence gates for truthful Linux capability advertisement.

## Requirements

### Requirement: Providers implement one reusable conformance contract
Every provider SHALL run through a shared harness covering compliant state, drift, Apply, second-check idempotence, absence, unsupported capability, probe failure, validation failure, lock contention, activation, redaction, rollback class, cancellation, and bounded timeout where applicable.

#### Scenario: Provider converges drift
<!-- verification-id: OS-LPC-001 -->
- **WHEN** the harness presents supported drift and Apply succeeds
- **THEN** the next Check reports compliant without a second mutation

#### Scenario: Provider cannot observe required state
<!-- verification-id: OS-LPC-002 -->
- **WHEN** its probe fails or returns ambiguous evidence
- **THEN** Check reports a typed failure or unsupported result rather than ordinary drift or false compliance

### Requirement: Contract tests use the public provider interface
The conformance harness SHALL construct providers through their supported interface and observe typed results and controlled external effects. It SHALL NOT depend on private helper functions or provider-specific internal call order.

#### Scenario: Provider implementation is reorganized
<!-- verification-id: OS-LPC-003 -->
- **WHEN** command assembly and helper packages change without changing provider behavior
- **THEN** the shared contract remains valid without provider-specific expectation rewrites

### Requirement: Capability matrix is versioned evidence
The repository SHALL maintain a versioned matrix of distribution release, architecture, relevant backend, provider contract revision, environment kind, and required test selectors for every advertised combination.

#### Scenario: New distribution version is proposed
<!-- verification-id: OS-LPC-004 -->
- **WHEN** no passing matrix row exists for that release and backend
- **THEN** Remotr does not advertise support until the row and required evidence are added

### Requirement: Advertisement is derived from passing conformance
A provider or provider option SHALL be advertised only when its shared contract tests and all required real-environment matrix tests pass. Schema acceptance alone SHALL NOT enable advertisement.

#### Scenario: Provider implementation exists without integration coverage
<!-- verification-id: OS-LPC-005 -->
- **WHEN** unit and command-boundary tests pass but the required distribution test does not exist
- **THEN** authored configuration is rejected or the endpoint reports unsupported according to the capability contract

### Requirement: Containers and VMs have explicit proof boundaries
Container environments SHALL be used only for behavior faithfully exposed by their namespaces, filesystem, and installed tools. Reboot, boot, network-control recovery, mount/kernel, MAC, authentication recovery, and destructive-device behavior SHALL use isolated VMs when containers cannot prove the contract.

#### Scenario: Network profile could sever control connectivity
<!-- verification-id: OS-LPC-006 -->
- **WHEN** the provider safety contract requires loss and restoration of the real management path
- **THEN** passing command mocks or an unprivileged container do not satisfy the required VM evidence

### Requirement: Negative recovery behavior is first-class
Provider environments SHALL intentionally exercise Remotr connectivity loss, SSH/sudo lockout risk, invalid boot state, secret canary leakage, ambiguous devices, lock contention, cancellation, and rollback failure as applicable to the provider risk class.

#### Scenario: Access change removes the recovery principal
<!-- verification-id: OS-LPC-007 -->
- **WHEN** the negative test attempts to remove the last verified administrative path
- **THEN** the provider blocks before mutation and reports the expected safety reason

### Requirement: Environment tests are isolated and reproducible
Provider tests SHALL use pinned images or VM definitions, synthetic credentials and secret canaries, isolated networks and disks, deterministic fixtures, and verified cleanup. No destructive or connectivity-loss test SHALL target a contributor host or shared production resource.

#### Scenario: Destructive VM scenario completes
<!-- verification-id: OS-LPC-008 -->
- **WHEN** the scenario passes or fails
- **THEN** its disposable disk, network, credentials, and retained secret material are destroyed and teardown is verified

### Requirement: Provider failures retain diagnosable artifacts
Failed environment tests SHALL retain bounded redacted logs, provider facts, commands/argv where safe, state transitions, and relevant system evidence without leaking secret values.

#### Scenario: Secret-backed repository test fails
<!-- verification-id: OS-LPC-009 -->
- **WHEN** diagnostic artifacts are uploaded
- **THEN** they identify the provider, step, and safe secret version metadata but contain no credential bytes or secret-bearing arguments

### Requirement: Current providers migrate before the gate becomes universal
The foundation SHALL migrate representative existing package, file, service, and firewall providers through the conformance harness, record gaps, and define a bounded migration sequence before requiring all advertised providers to pass.

#### Scenario: Legacy provider lacks one contract behavior
<!-- verification-id: OS-LPC-010 -->
- **WHEN** initial conformance reveals the gap
- **THEN** the matrix records it as unverified and the migration plan either fixes or truthfully de-advertises the behavior before the universal gate activates
