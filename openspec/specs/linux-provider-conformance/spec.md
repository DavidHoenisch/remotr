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
### Requirement: Exact distribution identity is distinct from family compatibility
Endpoint facts and provider selection SHALL preserve an exact operating-system identity separately from compatibility lineage. A provider scoped to Canonical Ubuntu SHALL require exact, consistent Ubuntu release and vendor evidence and SHALL NOT infer identity from `ID_LIKE`, package manager, init system, kernel, branding, or the presence of provider binaries.

#### Scenario: Derivative is compatible with Debian family
<!-- verification-id: OS-LPC-011 -->
- **WHEN** Pop!_OS or another derivative reports Debian or Ubuntu lineage and uses APT
- **THEN** generic family facts may select independently qualified family providers while Ubuntu-only capabilities remain absent

#### Scenario: Exact and compatibility sources conflict
<!-- verification-id: OS-LPC-012 -->
- **WHEN** operating-system identity sources disagree or exact product identity is ambiguous
- **THEN** the endpoint does not advertise an exact-distribution provider and that provider performs no mutation

### Requirement: Release-family claims remain capability-specific
Qualifying a provider on a newer distribution release SHALL advertise only the exact capability, backend, contract revision, release, architecture, environment, service, mode, variant, and disable-behavior row that passed. The system SHALL NOT extend sibling resource rows, infer specialized service support from base attachment, or interpret an inclusive release range as conformance evidence.

#### Scenario: Ubuntu Pro passes on Ubuntu 26.04
<!-- verification-id: OS-LPC-013 -->
- **WHEN** the Ubuntu Pro provider passes its Ubuntu 26.04 LTS amd64 VM row while another resource remains qualified only on 24.04
- **THEN** the endpoint advertises Ubuntu Pro on 26.04 and does not advertise the unrelated resource

#### Scenario: Latest label changes over time
<!-- verification-id: OS-LPC-014 -->
- **WHEN** Canonical publishes another LTS after the newest checked-in passing row
- **THEN** Remotr continues using the frozen matrix and does not advertise the new release until explicit evidence is added

#### Scenario: Base subscription attachment passes
<!-- verification-id: OS-LPC-017 -->
- **WHEN** an Ubuntu release passes the base attachment contract but a service, mode, variant, or purge selector has not passed
- **THEN** the endpoint advertises attachment only and artifact delivery requiring the unproven tuple remains blocked

#### Scenario: Service option passes on one exact row
<!-- verification-id: OS-LPC-018 -->
- **WHEN** a service option passes on one release, architecture, client API revision, and environment row
- **THEN** capability generation advertises only that exact tuple and does not generalize it to sibling services, modes, variants, releases, or architectures

### Requirement: Subscription providers require host VM and deterministic API evidence
A provider that attaches a host subscription or changes entitled operating-system services SHALL use disposable, pinned real Ubuntu VMs for each advertised release and architecture. Repeatable qualification SHALL use deterministic, independently specified Ubuntu Pro API doubles at the external process boundary instead of a live Canonical subscription token. Evidence SHALL include public provider-contract behavior, exact process requests, protected synthetic credential injection, negative derivative checks, dependency and incompatibility handling, recovery, cleanup, and secret-canary absence. It SHALL NOT claim that CI attached a live subscription or observed entitled package, snap, repository, kernel, or compliance-tool effects.

#### Scenario: Unit-only subscription test passes
<!-- verification-id: OS-LPC-015 -->
- **WHEN** a unit test or container-only mocked client passes attachment command tests without the pinned Ubuntu host lifecycle and public provider seam
- **THEN** no Ubuntu Pro provider row becomes advertised

#### Scenario: Credential-free VM fixture completes
<!-- verification-id: OS-LPC-016 -->
- **WHEN** a release fixture succeeds or fails using a protected synthetic token and deterministic API responses on a pinned Ubuntu VM
- **THEN** it verifies provider detach and recovery behavior where applicable, destroys the VM and synthetic credential-bearing runtime state, retains only bounded redacted evidence, and records that no live Canonical subscription was exercised

### Requirement: Provider evidence proves the versioned API boundary
An Ubuntu Pro provider row SHALL prove the exact `/usr/bin/pro api <endpoint>` request contract, the common JSON envelope, and protected JSON stdin for parameterized calls. Deterministic API doubles SHALL be driven by independently specified request and response fixtures and exercised through the public provider seam; assertions limited to private collaborator call order SHALL NOT qualify a row. Evidence based on ordinary Ubuntu Pro command output, generic argument construction, or a successful mutation without a second observable Check SHALL NOT qualify a row. A required versioned endpoint that is unavailable SHALL remain unsupported without legacy fallback.

#### Scenario: API contract test passes but ordinary CLI fallback exists
<!-- verification-id: OS-LPC-019 -->
- **WHEN** a provider can invoke the documented API but would fall back to ordinary `pro status`, `enable`, `disable`, or `attach` after an endpoint error
- **THEN** its selector fails and no affected capability row is advertised

#### Scenario: Parameterized API evidence is collected
<!-- verification-id: OS-LPC-020 -->
- **WHEN** an attachment or service mutation row is evaluated
- **THEN** evidence asserts literal endpoint argv, bounded JSON stdin, stable envelope-code parsing, secret redaction, Apply, second Check, and no use of `--args` or localized message text for control flow

#### Scenario: Specialized service lacks durable observation
<!-- verification-id: OS-LPC-022 -->
- **WHEN** a service or mode can be invoked but its desired state cannot be re-observed through a stable API field or separately reviewed native seam
- **THEN** the row remains unadvertised and a one-time success response does not qualify declarative support

### Requirement: Production capability catalogs are frozen evidence outputs
Every production agent release SHALL contain a canonical immutable capability catalog generated deterministically from checked-in passing qualification rows. The production default capability generator SHALL evaluate that embedded catalog against normalized runtime facts. Test-only constructor wiring, runtime reads from test fixtures, implementation presence, executable presence, and remotely fetched data SHALL NOT add advertised capabilities. Agent release metadata used by the server SHALL be generated from the same release inputs and SHALL distinguish protocol or binary eligibility from runtime provider evidence.

#### Scenario: Passing Ubuntu Pro manifest is packaged
<!-- verification-id: OS-LPC-023 -->
- **WHEN** an exact Ubuntu Pro qualification row passes all required selectors and is included in a release
- **THEN** the production default generator advertises only that row on matching runtime facts without requiring a test-only constructor or runtime test-file access

#### Scenario: Generated catalog is stale or conflicting
<!-- verification-id: OS-LPC-024 -->
- **WHEN** regeneration changes packaged output, a row lacks required evidence selectors, duplicate rows conflict, or server release metadata disagrees with the agent payload
- **THEN** validation or release packaging fails and no affected row is published

#### Scenario: Agent version is known to the server
<!-- verification-id: OS-LPC-025 -->
- **WHEN** the server recognizes an approved agent release and considers an upgrade for a blocked endpoint
- **THEN** release metadata may prove binary and protocol eligibility but does not prove that the endpoint runtime satisfies a provider capability

### Requirement: A target release is advertised only with complete applicable provider evidence
Before Remotr advertises support for a distribution release and architecture in a representative public configuration, every resource and provider row applicable to that target SHALL either have its own complete required evidence and frozen catalog entry or SHALL be rejected at author-time validation. Evidence for a specialized provider SHALL NOT promote sibling core providers, and evidence for a core provider SHALL NOT promote specialized capabilities.

#### Scenario: Ubuntu Pro passes but a core provider does not
<!-- verification-id: OS-LPC-026 -->
- **WHEN** Ubuntu Pro passes on Ubuntu 26.04 amd64 but an applicable file, command, package, init, download, or other core provider row lacks its governing evidence
- **THEN** only Ubuntu Pro's passing rows may be advertised and validation rejects configuration requiring the unqualified core row

#### Scenario: Representative target inventory is complete
<!-- verification-id: OS-LPC-027 -->
- **WHEN** every row required by the Ubuntu 26.04 amd64 public qualification configuration has passed its own selected provider, safety, redaction, and cleanup evidence
- **THEN** the frozen catalog contains exactly those applicable rows and authenticated Sync can evaluate the configuration without manufacturing support

#### Scenario: Ubuntu 24.04 core delivery contracts are qualified
<!-- verification-id: OS-LPC-029 -->
- **WHEN** command, bootstrap, and systemd pass their provider contracts on the pinned Ubuntu 24.04 LTS amd64 VM and the production capability document is generated from exact Ubuntu 24.04 amd64, systemd, and APT facts
- **THEN** the frozen catalog advertises `command-v1`, `bootstrap-v1`, and `systemd-v1` only for that exact target without manufacturing support for another release or architecture
