## ADDED Requirements

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
