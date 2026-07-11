## ADDED Requirements

### Requirement: Versioned and strict desired-state input
The system SHALL identify the canonical composed-artifact schema version and SHALL reject unknown resource fields, unknown resource kinds, invalid enum values, and invalid field combinations before releasing or applying an artifact.

#### Scenario: Unknown field is rejected
- **WHEN** a configuration contains a field not defined by its declared schema version
- **THEN** validation fails with the configuration name, resource address, and unknown field

#### Scenario: Compatible legacy input is rendered canonically
- **WHEN** a supported legacy resource is read during the compatibility window
- **THEN** composition emits its canonical equivalent and a deprecation diagnostic without changing the requested outcome

### Requirement: Accepted fields are convergent
The system SHALL check and apply every accepted managed field. An omitted optional field SHALL mean unmanaged unless the field's specification declares a default.

#### Scenario: Managed metadata drifts
- **WHEN** a resource's content is correct but one accepted managed metadata field differs
- **THEN** Check reports drift and Apply repairs that metadata field

#### Scenario: Field is unavailable in a provider
- **WHEN** a resource requests a field that the selected provider cannot check and apply
- **THEN** the artifact is rejected as an unsupported field/provider combination rather than silently ignoring the field

### Requirement: Structured check outcomes
Each applicator SHALL return exactly one check status from `compliant`, `drifted`, `unsupported`, `check_failed`, or `deferred`, together with a stable reason code and redacted desired and observed summaries when applicable.

#### Scenario: Probe command fails
- **WHEN** an applicator cannot observe state because its probe fails
- **THEN** the result is `check_failed` and is not counted as either compliance or drift

#### Scenario: Provider is unavailable
- **WHEN** no compatible provider is present on the endpoint
- **THEN** the result is `unsupported` and identifies the required capability without entering an apply loop

### Requirement: Apply eligibility follows check status
The engine SHALL apply only resources whose status is `drifted`, whose dependencies are satisfied, whose policy permits remediation, and whose safety preconditions pass.

#### Scenario: Unsupported resource is not applied
- **WHEN** Check returns `unsupported`
- **THEN** the engine does not invoke Apply and reports the unsupported result

#### Scenario: Report policy observes drift
- **WHEN** Check returns `drifted` under report-only policy
- **THEN** the resource remains unchanged and the report records remediation as skipped by policy

### Requirement: Stable identity and dependency behavior
Every resource SHALL have a stable `<configuration>/<resource-name>` address, and names SHALL be unique across all resource kinds within one configuration. Dependencies SHALL reference stable addresses and SHALL prevent dependent application after failure, unsupported state, or deferral.

#### Scenario: Cross-kind duplicate name
- **WHEN** two resources of different kinds share a name in one configuration
- **THEN** validation fails because their stable address would be ambiguous

#### Scenario: Dependency apply fails
- **WHEN** a dependency fails during Apply
- **THEN** dependent resources are reported blocked and are not applied

### Requirement: Provider capability negotiation
The system SHALL select providers from normalized endpoint facts and an advertised capability matrix. Statically impossible target/provider combinations SHALL fail author-time validation; endpoint-local mismatches SHALL produce `unsupported`.

#### Scenario: RPM provider targeted before support
- **WHEN** configuration targets an RPM-family package provider not advertised by the target agent matrix
- **THEN** configuration validation rejects the release

#### Scenario: Expected backend binary is missing
- **WHEN** the artifact is valid for the endpoint facts but the required backend is absent locally
- **THEN** Check returns `unsupported` with the backend capability reason

### Requirement: Explicit ownership and lifecycle
Each resource kind SHALL declare whether it owns one named object, one Remotr-owned fragment, or an authoritative set. Absence or garbage collection SHALL occur only when explicitly requested by lifecycle or authoritative ownership.

#### Scenario: Resource omitted from later artifact
- **WHEN** a previously managed resource is omitted and no authoritative-set contract covers it
- **THEN** the endpoint state is left unchanged

#### Scenario: Authoritative set removes extra managed member
- **WHEN** an authoritative set contains fewer members than the prior desired set
- **THEN** Apply removes only members inside that set's declared ownership boundary

### Requirement: Risk classes and preflight
Resource kinds SHALL declare their risk class. Connectivity, access, boot, destructive, and sensitive changes SHALL require kind-specific preflight and SHALL default to non-enforcing behavior where their capability specification requires it.

#### Scenario: High-risk preflight fails
- **WHEN** a high-risk resource fails syntax, recovery, connectivity, capacity, or other required preflight
- **THEN** Apply is blocked without activating partial state and the structured reason is reported

### Requirement: Exclusive operation locks
The agent SHALL serialize resources that share an exclusive lock domain and SHALL honor provider-native locks with bounded waits.

#### Scenario: Package database is busy
- **WHEN** the native package database lock cannot be acquired before the configured timeout
- **THEN** Apply returns a retryable lock failure without starting a competing transaction

### Requirement: Structured activation outcomes
Applicators SHALL return activation needs as structured signals, and the engine SHALL order and coalesce compatible reload, restart, daemon-reload, logout, next-boot, and reboot-required signals.

#### Scenario: Multiple unit fragments change
- **WHEN** multiple resources require the same daemon reload in one successful run
- **THEN** the engine performs one correctly ordered daemon reload before dependent service activation

### Requirement: Honest rollback reporting
Each resource SHALL advertise rollback as `transactional`, `best_effort`, or `none`. Apply failures SHALL preserve the original error and report rollback outcome separately.

#### Scenario: Apply and rollback both fail
- **WHEN** Apply fails and its best-effort rollback also fails
- **THEN** reporting retains both failures and does not replace the apply error with the rollback error

### Requirement: Typed redaction
The schema SHALL classify fields as public, sensitive metadata, or secret, and secret values SHALL be redacted before entering logs, reports, diffs, backups, diagnostics, or persistent telemetry.

#### Scenario: Secret-backed resource drifts
- **WHEN** Check evaluates a secret-backed resource
- **THEN** the report may include its reference, safe fingerprint, and health but never the secret value

### Requirement: Fleet state distinguishes outcomes
Endpoint and fleet reports SHALL separately aggregate compliant, drifted, unsupported, check-failed, deferred, apply-failed, and no-report states.

#### Scenario: Fleet contains mixed outcomes
- **WHEN** a fleet has endpoints in drift, unsupported, and no-report states
- **THEN** each endpoint and each fleet summary bucket reflects its actual state without labeling unsupported endpoints as drifted

