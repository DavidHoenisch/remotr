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

### Requirement: Secret-bearing subscription providers require host VM evidence
A provider that attaches a host subscription or changes entitled operating-system services SHALL use disposable, pinned real Ubuntu VMs for each advertised release and architecture. Behavior-specific fixtures SHALL be used where a service changes snaps, kernels, boot configuration, compliance tooling, external registration, or package purge. Evidence SHALL include protected credential injection, public provider-contract behavior, exact process boundaries, native service/package/boot effects, negative derivative checks, dependency and incompatibility handling, recovery, cleanup, and secret-canary absence.

#### Scenario: Container-only subscription test passes
<!-- verification-id: OS-LPC-015 -->
- **WHEN** a container or mocked client passes attachment command tests without a real supported Ubuntu host lifecycle
- **THEN** no Ubuntu Pro provider row becomes advertised

#### Scenario: VM fixture completes
<!-- verification-id: OS-LPC-016 -->
- **WHEN** a release fixture succeeds or fails after using a protected test subscription
- **THEN** it verifies detach where applicable, destroys the VM and credential-bearing runtime state, and retains only bounded redacted evidence

### Requirement: Provider evidence proves the versioned API boundary
An Ubuntu Pro provider row SHALL prove the exact `/usr/bin/pro api <endpoint>` process boundary, the common JSON envelope, and protected JSON stdin for parameterized calls. Evidence based on ordinary Ubuntu Pro command output, generic argument construction, a Remotr-owned mock interaction, or a successful mutation without a second observable Check SHALL NOT qualify a row. A required versioned endpoint that is unavailable SHALL remain unsupported without legacy fallback.

#### Scenario: API contract test passes but ordinary CLI fallback exists
<!-- verification-id: OS-LPC-019 -->
- **WHEN** a provider can invoke the documented API but would fall back to ordinary `pro status`, `enable`, `disable`, or `attach` after an endpoint error
- **THEN** its selector fails and no affected capability row is advertised

#### Scenario: Parameterized API evidence is collected
<!-- verification-id: OS-LPC-020 -->
- **WHEN** an attachment or service mutation row is evaluated
- **THEN** evidence asserts literal endpoint argv, bounded JSON stdin, stable envelope-code parsing, secret redaction, Apply, second Check, and no use of `--args` or localized message text for control flow

### Requirement: Non-API exceptions are isolated and independently qualified
When Canonical exposes no versioned API for a desired stable service, the service SHALL use a separate typed provider contract or remain unsupported. An exception SHALL define fixed inputs, protected secret transport, observable convergence state, exact process boundaries, safety and cleanup behavior, and its own provider-matrix rows. It SHALL NOT weaken the API-first boundary for any other Ubuntu Pro service.

#### Scenario: Landscape uses a dedicated provider contract
<!-- verification-id: OS-LPC-021 -->
- **WHEN** the generic service API reports Landscape as unsupported but a typed Landscape row has complete native and external-state evidence
- **THEN** only that exact Landscape row may advertise, using its fixed contract without exposing generic CLI arguments

#### Scenario: Specialized service lacks durable observation
<!-- verification-id: OS-LPC-022 -->
- **WHEN** a service or mode can be invoked but its desired state cannot be re-observed through a stable API field or separately reviewed native seam
- **THEN** the row remains unadvertised and a one-time success response does not qualify declarative support
