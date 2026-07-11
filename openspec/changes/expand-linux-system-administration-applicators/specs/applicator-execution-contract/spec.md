## ADDED Requirements

### Requirement: Advertised behavior has traceable verification evidence
Every advertised field, provider, status, workflow, and compatibility behavior SHALL have an immutable OpenSpec verification ID and passing evidence recorded through the `establish-testing-and-performance-foundation` traceability contract. Evidence SHALL use the cheapest trustworthy public seam and SHALL include real provider, VM safety, mutation, or performance layers when required by risk. A `planned`, missing, deferred-only, skipped, or failing disposition SHALL NOT authorize advertisement.

#### Scenario: New provider has command-mock coverage only
- **WHEN** its schema and command-boundary unit tests pass but its required conformance and distribution matrix evidence is absent
- **THEN** the provider remains unadvertised and implementation tasks governing its release remain incomplete

#### Scenario: Cross-component safety workflow is verified
- **WHEN** its traceability entry points to passing authorization, Godog acceptance, provider, and VM recovery evidence required by its risk class
- **THEN** the release gate may count the scenario as verified without requiring duplicate tests at every other layer

#### Scenario: Existing evidence is removed
- **WHEN** a change deletes or weakens the only passing selector for an active advertised scenario
- **THEN** traceability and release validation fail until replacement evidence or an approved specification disposition exists

### Requirement: Versioned and strict desired-state input
The system SHALL identify canonical deployable artifacts with top-level integer `schemaVersion: 1`. During the compatibility window, an unversioned artifact SHALL be interpreted as legacy schema `0`. Schema `1` validation SHALL reject unknown resource fields, unknown resource kinds, invalid enum values, and invalid field combinations before releasing or applying an artifact.

#### Scenario: Unknown field is rejected
- **WHEN** a configuration contains a field not defined by its declared schema version
- **THEN** validation fails with the configuration name, resource address, and unknown field

#### Scenario: Compatible legacy input is rendered canonically
- **WHEN** a supported legacy resource is read during the compatibility window
- **THEN** it is interpreted as schema `0`, composition emits its schema-`1` canonical equivalent for an eligible fleet, and tooling emits a deprecation diagnostic without changing the requested outcome

#### Scenario: Legacy compatibility expires safely
- **WHEN** fewer than two minor releases or 90 days have elapsed since schema `1` shipped, or fleet telemetry still reports schema-`0` endpoints
- **THEN** the system continues to accept schema `0`

#### Scenario: Legacy compatibility is removed
- **WHEN** both minimum support periods have elapsed, telemetry reports no schema-`0` endpoints, and a breaking release announced removal
- **THEN** schema `0` may be rejected with a migration diagnostic

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
Every modern agent SHALL report an authenticated endpoint capability document on every Sync containing its supported artifact schema versions, stable capability IDs and contract revisions, normalized provider facts, agent version, and document digest. The server SHALL record its own receive time, store the latest document for readiness/reporting, and select an artifact using only capability evidence from the current authenticated Sync. Agent version SHALL NOT by itself prove runtime provider support. Statically impossible target/provider combinations SHALL fail author-time validation; endpoint-local mismatches SHALL produce `unsupported`.

#### Scenario: RPM provider targeted before support
- **WHEN** configuration targets an RPM-family package provider not advertised by the target agent matrix
- **THEN** configuration validation rejects the release

#### Scenario: Expected backend binary is missing
- **WHEN** the artifact is valid for the endpoint facts but the required backend is absent locally
- **THEN** Check returns `unsupported` with the backend capability reason

#### Scenario: Legacy agent reports no capability document
- **WHEN** a known legacy agent version syncs without a capability document
- **THEN** the server assigns only the conservative schema-`0` capability profile mapped to that version

#### Scenario: Unknown agent reports no capability document
- **WHEN** an unknown agent version syncs without a capability document
- **THEN** the server assumes only the legacy baseline and fails closed for newer capabilities

#### Scenario: Modern agent omits current capability evidence
- **WHEN** an agent version that implements capability reporting omits or sends an invalid document on Sync
- **THEN** the server keeps its last active artifact and reports capability-blocked instead of selecting from persisted evidence

#### Scenario: Endpoint reconnects after being offline
- **WHEN** an endpoint's persisted capability document is old and the endpoint reconnects with a valid current document
- **THEN** selection uses the current document without applying an arbitrary wall-clock freshness threshold

### Requirement: Artifact delivery is capability-compatible
Composition SHALL calculate an explicit artifact requirement set and SHALL cache only bounded schema variants: canonical schema `1` and, during migration, a schema-`0` variant when it is behaviorally lossless. On Sync, the server SHALL serve the highest variant satisfied by the endpoint capability document and SHALL NOT omit resources or fields to manufacture compatibility.

#### Scenario: Legacy variant is lossless
- **WHEN** the current desired state can be represented in schema `0` without losing behavior and an endpoint supports only schema `0`
- **THEN** the server serves the schema-`0` compatibility variant for the current Release ref

#### Scenario: No compatible current artifact exists
- **WHEN** the current Release ref requires capabilities absent from an existing endpoint
- **THEN** the global Release ref advances, the endpoint continues checking its last successfully processed artifact, and Sync reports `capability_blocked` with the unavailable Release ref and missing capabilities

#### Scenario: New endpoint has no compatible artifact
- **WHEN** a newly enrolled endpoint has no prior artifact and cannot satisfy any current variant
- **THEN** it remains explicitly unmanaged and `capability_blocked` rather than receiving partial desired state

#### Scenario: Compatible agent upgrade is available
- **WHEN** an endpoint is capability-blocked and an approved agent version satisfies the missing contract
- **THEN** the server may include that agent-upgrade instruction without claiming the current artifact is active

### Requirement: Artifact release state is observable
Operator reporting SHALL distinguish the global available Release ref, the endpoint's active artifact Release ref, its artifact schema version, and any capability-blocked target Release ref.

#### Scenario: Endpoint remains on an older artifact
- **WHEN** the global Release ref advances but an endpoint is capability-blocked
- **THEN** state reporting shows both refs and does not attribute checks against the old active artifact to the newer Release ref

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

### Requirement: High-risk changes have a reviewable request
When a merged Release ref changes related high-risk resources, the server SHALL create a Change request containing the exact Release ref, artifact and resource hashes, risk classes, frozen evaluated endpoint set, compatibility, predicted effects, rollback capability, and per-endpoint preflight evidence. High-risk Apply SHALL remain blocked until the request has matching authorization.

#### Scenario: Normal resource accompanies pending high-risk resource
- **WHEN** a Release ref contains both normal and unauthorized high-risk drift
- **THEN** normal resources may converge while high-risk resources and their dependents remain non-enforcing

#### Scenario: Endpoint joins after rollout target freeze
- **WHEN** a new endpoint joins after a Rollout authorization's target set was frozen
- **THEN** it is not added to that rollout authorization

### Requirement: Authorization grouping is explicit and bounded
Resources MAY declare `authorizationGroup` to coordinate review and rollout of related high-risk state. A Change request SHALL NOT span Fleets or endpoint overrides. Without an explicit group, the server SHALL group high-risk resources by dependency-connected component, including shared activation edges; independent components SHALL produce separate requests. A mixed-risk group SHALL inherit its strictest authorization and rollout policy.

#### Scenario: Explicit network transition group
- **WHEN** DNS, route, firewall, and network-profile resources share one authorization group in a Fleet
- **THEN** the server creates one Change request containing their combined plan and strictest applicable risk policy

#### Scenario: Unrelated high-risk change shares a Release ref
- **WHEN** an independent sudo-policy resource changes in the same Release ref as a grouped network transition
- **THEN** the server creates a separate Change request for the sudo-policy component

#### Scenario: Same group name appears in two Fleets
- **WHEN** resources in two Fleets use the same authorization group name
- **THEN** each Fleet receives a separate Change request and authorization history

#### Scenario: Normal prerequisite is included in the plan
- **WHEN** a high-risk resource depends on a normal package or file resource
- **THEN** the prerequisite appears in the Change request plan but retains its normal risk class

### Requirement: Rollout authorization is temporary and exact
A Rollout authorization SHALL bind the exact desired-state hashes to a frozen endpoint set, maintenance window, expiry, attempt limit, concurrency policy, approving operator, and justification.

#### Scenario: Desired state changes after authorization
- **WHEN** any authorized high-risk resource field changes
- **THEN** its desired-state hash no longer matches and a new Change request is required

### Requirement: Approval thresholds are configurable by risk
The server registry SHALL define global Approval policy with optional Fleet and risk-class overrides. By default, connectivity, access/privilege, boot policy, coordinated reboot, and their baseline promotion SHALL require one approver, while destructive storage SHALL require two. Approvals SHALL count distinct operator identities with appropriate RBAC permission rather than distinct credentials.

#### Scenario: Operator has two credentials
- **WHEN** the same operator identity approves through two valid certificates
- **THEN** those approvals count once toward the threshold

#### Scenario: Destructive change has one approval
- **WHEN** a destructive-storage Change request has only one approval under default policy
- **THEN** authorization remains pending at one of two approvals

#### Scenario: Small installation lowers destructive threshold
- **WHEN** a single-operator installation explicitly configures one destructive approver
- **THEN** the server permits the policy and continuously exposes an audit warning

### Requirement: Approval is bound to an unchanged plan
An approval SHALL bind resource hashes, target scope, authorization lifetime, maintenance constraints, and rollout-control bounds. Changing any bound field SHALL invalidate collected approvals.

#### Scenario: Concurrency increases after approval
- **WHEN** an operator raises maximum concurrency beyond the approved bound
- **THEN** prior approvals are invalidated and the authorization returns to approval-pending

#### Scenario: Baseline promotion was approved initially
- **WHEN** the original authorization explicitly includes baseline-on-success and satisfies its risk policy
- **THEN** successful verified rollout may promote the exact hashes without a second approval round

#### Scenario: Baseline promotion is requested later
- **WHEN** baseline promotion was not included in the rollout authorization
- **THEN** the promotion must satisfy the current Approval policy with fresh approvals

### Requirement: Proven state can become a fleet baseline
A baseline-eligible high-risk resource SHALL be promotable to a Fleet baseline authorization only after successful verified rollout and explicit authorization. The baseline SHALL bind fleet, resource address, desired-state hash, risk class, and provider constraints. A future fleet member SHALL apply it automatically only after passing current capability and safety preflight.

#### Scenario: New endpoint matches an approved baseline
- **WHEN** a later-enrolled endpoint belongs to the fleet, resolves the same resource hash, and passes local preflight
- **THEN** it may converge without replaying the historical Rollout authorization

#### Scenario: New endpoint fails baseline preflight
- **WHEN** a later-enrolled endpoint resolves an approved baseline but fails provider or safety preflight
- **THEN** only that endpoint is blocked and the baseline authorization remains valid for other compatible endpoints

### Requirement: Authorization validity and execution windows are separate
Rollout authorization SHALL have configurable validity with a 30-day default and MAY contain recurring execution windows. A frozen target SHALL execute only while authorization is valid, an execution window is open, and its current preflight passes.

#### Scenario: Offline endpoint reconnects during a later window
- **WHEN** a frozen target was offline during the first window but reconnects during a later window before authorization expiry
- **THEN** it may execute after a fresh successful preflight without new approval

#### Scenario: Endpoint reconnects after authorization expiry
- **WHEN** a frozen target reconnects after Rollout authorization expires and no Fleet baseline authorization exists
- **THEN** it remains non-enforcing until a new authorization is approved

### Requirement: High-risk work requires an endpoint execution lease
The server SHALL issue a short-lived Execution lease only when the endpoint is in the frozen target set, current preflight passes, authorization remains valid, an Execution window is open, and a concurrency slot is available. The lease SHALL bind Change request, endpoint, resource hashes, attempt, and expiry and SHALL be delivered during authenticated Sync. The agent SHALL NOT begin high-risk work from cached authorization alone.

#### Scenario: Concurrency limit is reached
- **WHEN** all approved concurrency slots are leased
- **THEN** another ready endpoint remains scheduled until a slot becomes available

#### Scenario: Change request is paused
- **WHEN** an operator pauses a Change request
- **THEN** the server issues no new leases while already applying endpoints continue to verification or rollback

#### Scenario: Cached lease has expired
- **WHEN** an endpoint has not begun work before its lease expiry
- **THEN** it discards the lease and requests a new one on a later Sync

### Requirement: Acknowledgement is risk-specific
Each high-risk provider SHALL define preparation, verification, acknowledgement, timeout, and rollback behavior appropriate to its risk. Progress SHALL report lease-issued, prepared, applying, verifying, acknowledged, rolled-back, failed, or acknowledgement-timeout.

#### Scenario: Network transition reconnects
- **WHEN** a network or firewall change completes a new authenticated Sync over the changed path before timeout
- **THEN** the server acknowledges the attempt and the endpoint cancels its armed rollback

#### Scenario: Network transition loses control path
- **WHEN** no authenticated Sync acknowledgement arrives before timeout
- **THEN** the provider checkpoint or independent local watchdog restores prior network state and reports rollback outcome

#### Scenario: Access policy passes local validation
- **WHEN** SSH, sudo, or PAM effective validation passes and Remotr Sync remains available
- **THEN** the endpoint may report technical verification while clearly not claiming that a human login was tested

#### Scenario: Reboot reconnects with a new boot ID
- **WHEN** a leased reboot acknowledges intent and later reconnects with a different boot ID
- **THEN** the attempt is acknowledged as successful

#### Scenario: Irreversible storage succeeds
- **WHEN** an authorized destructive operation satisfies exact device preconditions and postconditions
- **THEN** it reports verified success and rollback capability `none` rather than inventing a revert path

### Requirement: Emergency authorization is explicit and bounded
Emergency operation SHALL use a dedicated Break-glass authorization requiring risk-specific RBAC permission, exact resource hashes and targets, justification, external incident/change reference, bounded validity, and attempt count. It MAY bypass normal approval count, Execution windows, and concurrency but SHALL NOT bypass schema/provider validation, redaction, current preflight, required rollback, or destructive-storage identity and irreversible-operation approval.

#### Scenario: Endpoint break glass uses defaults
- **WHEN** an authorized operator creates break glass without wider bounds
- **THEN** it applies to one endpoint, one attempt, and at most 60 minutes

#### Scenario: Fleet-wide break glass is requested
- **WHEN** break glass targets an entire Fleet
- **THEN** explicit fleet scope and a second distinct operator are required

#### Scenario: Break glass targets destructive storage
- **WHEN** an operator attempts to bypass stable device identity or irreversible-operation approval
- **THEN** the request is rejected even with break-glass permission

#### Scenario: Break glass lifecycle is audited
- **WHEN** break glass is created, used, expires, or is revoked
- **THEN** the server emits a prominent audit event suitable for SIEM alerting

#### Scenario: Server is unreachable
- **WHEN** an operator cannot reach the Remotr server
- **THEN** break glass is unavailable and documentation directs the operator to out-of-band recovery

### Requirement: Rollout evidence accounts for every frozen target
Change request reporting SHALL classify every frozen target as verified successful, failed or rolled back, capability or preflight blocked, or not seen during authorization.

#### Scenario: Laptop remains offline throughout rollout
- **WHEN** a frozen target never Syncs during authorization validity
- **THEN** the completed rollout reports it as not seen rather than successful or failed

### Requirement: Baseline promotion is manual by default
Manual baseline promotion SHALL be the default. Offline, blocked, and not-seen targets SHALL NOT prevent manual promotion, but the operator SHALL see and explicitly acknowledge exceptions under the applicable Approval policy.

#### Scenario: Operator promotes with offline exceptions
- **WHEN** verified endpoints succeeded, no unresolved failure is hidden, and the operator acknowledges not-seen endpoints with required approvals
- **THEN** the exact resource hashes become the Fleet baseline and the remaining endpoints must pass current preflight before applying

### Requirement: Automatic baseline promotion is policy-defined
Automatic baseline-on-success SHALL be unavailable unless Fleet Approval policy explicitly defines canary stages, minimum successful evidence, and maximum failures. Any unresolved failure or rollback SHALL block automatic promotion; the system SHALL NOT use a hidden universal percentage threshold.

#### Scenario: Automatic promotion has unresolved rollback
- **WHEN** the configured success threshold is otherwise met but any target has an unresolved rollback
- **THEN** automatic baseline promotion remains blocked

### Requirement: Event and destructive authorization do not become baselines
Reboot events, destructive storage grants, and emergency overrides SHALL NOT be promotable to Fleet baseline authorization. Destructive storage SHALL remain bound to endpoint-specific stable device identity.

#### Scenario: New endpoint joins after a reboot rollout
- **WHEN** a fleet reboot completed before the endpoint enrolled
- **THEN** the endpoint does not replay that reboot from baseline history

### Requirement: Existing fleets can adopt one reviewed baseline
The system SHALL support aggregating the currently resolved high-risk state of an existing fleet into one preflighted baseline-adoption request without creating a separate historical request for every resource.

#### Scenario: Fleet enables authorization feature
- **WHEN** an existing fleet already contains many unchanged high-risk resources
- **THEN** an operator can review and authorize their exact current hashes as one baseline adoption

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

### Requirement: Rollback storage is centralized and bounded
Rollback metadata and payloads SHALL be stored only in a root-owned agent directory keyed by Resource address, artifact digest, and attempt; applicators SHALL NOT leave generic adjacent backup files. Metadata SHALL retain at most 10 attempts per Resource and 30 days. Non-secret payloads SHALL retain at most three successful prior states and 30 days. Sensitive or secret payloads SHALL remain only while armed or unacknowledged and SHALL have an absolute 24-hour maximum. A configurable global disk cap SHALL also apply.

#### Scenario: Adjacent managed file is replaced
- **WHEN** a file provider stages rollback for `/etc/example.conf`
- **THEN** it stores protected rollback under agent state and does not create `/etc/example.conf.remotr.bak`

#### Scenario: Disk cap would remove armed rollback
- **WHEN** available rollback capacity cannot retain an armed payload
- **THEN** Apply is blocked before mutation rather than pruning the armed recovery state

### Requirement: Durable rollback payloads are encrypted
Durable rollback payloads SHALL use authenticated encryption under an endpoint-local rollback key. TPM sealing SHALL be preferred when available; a root-only key file MAY be used with reporting that it does not protect against endpoint-root compromise. Payloads SHALL be checksummed and written atomically.

#### Scenario: TPM is unavailable
- **WHEN** the endpoint has no supported TPM sealing provider
- **THEN** rollback may use a root-only local key while capability/reporting states the reduced protection

### Requirement: Secret rollback prefers version references
Server-managed secret rollback SHALL retain a prior Secret-version reference rather than generic secret bytes whenever online resolution can satisfy recovery. Disconnected recovery MAY retain encrypted prior bytes only until acknowledgement, with the sensitive-payload retention bound. A referenced version SHALL NOT be deleted until recovery expires or an authorized operator abandons it.

#### Scenario: Wi-Fi transition is awaiting acknowledgement
- **WHEN** rollback must operate after loss of server connectivity
- **THEN** the endpoint retains encrypted prior credential bytes only until acknowledgement, rollback, or the 24-hour maximum

#### Scenario: Operator deletes a referenced prior version
- **WHEN** a Secret version remains referenced by armed or retained rollback
- **THEN** deletion is blocked unless an authorized operator explicitly abandons that recovery path

### Requirement: Rollback cleanup follows terminal state
Rollback payloads SHALL be cleaned after successful acknowledgement, completed rollback, expiry, or supersession according to sensitivity and retention policy. The server SHALL store only rollback metadata and outcomes.

#### Scenario: Transaction acknowledges successfully
- **WHEN** a sensitive transactional change receives final acknowledgement
- **THEN** its local secret-bearing payload is destroyed promptly while non-secret audit metadata remains within retention bounds

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
