# Applicator Execution Contract Specification

## Purpose

Define the common desired-state, execution-safety, capability-delivery, and evidence-backed qualification contract for endpoint applicators.

## Requirements

### Requirement: Advertised behavior has traceable verification evidence
Every advertised field, provider, status, workflow, and compatibility behavior SHALL have an immutable OpenSpec verification ID and passing evidence recorded through the `establish-testing-and-performance-foundation` traceability contract. Evidence SHALL use the cheapest trustworthy public seam and SHALL include real provider, VM safety, mutation, or performance layers when required by risk. A `planned`, missing, deferred-only, skipped, or failing disposition SHALL NOT authorize advertisement.

#### Scenario: New provider has command-mock coverage only
<!-- verification-id: OS-AEC-001 -->
- **WHEN** its schema and command-boundary unit tests pass but its required conformance and distribution matrix evidence is absent
- **THEN** the provider remains unadvertised and implementation tasks governing its release remain incomplete

#### Scenario: Cross-component safety workflow is verified
<!-- verification-id: OS-AEC-002 -->
- **WHEN** its traceability entry points to passing authorization, Godog acceptance, provider, and VM recovery evidence required by its risk class
- **THEN** the release gate may count the scenario as verified without requiring duplicate tests at every other layer

#### Scenario: Existing evidence is removed
<!-- verification-id: OS-AEC-003 -->
- **WHEN** a change deletes or weakens the only passing selector for an active advertised scenario
- **THEN** traceability and release validation fail until replacement evidence or an approved specification disposition exists

### Requirement: Versioned and strict desired-state input
The system SHALL identify canonical deployable artifacts with top-level integer `schemaVersion: 1`. During the compatibility window, an unversioned artifact SHALL be interpreted as legacy schema `0`. Schema `1` validation SHALL reject unknown resource fields, unknown resource kinds, invalid enum values, and invalid field combinations before releasing or applying an artifact.

#### Scenario: Unknown field is rejected
<!-- verification-id: OS-AEC-004 -->
- **WHEN** a configuration contains a field not defined by its declared schema version
- **THEN** validation fails with the configuration name, resource address, and unknown field

#### Scenario: Compatible legacy input is rendered canonically
<!-- verification-id: OS-AEC-005 -->
- **WHEN** a supported legacy resource is read during the compatibility window
- **THEN** it is interpreted as schema `0`, composition emits its schema-`1` canonical equivalent for an eligible fleet, and tooling emits a deprecation diagnostic without changing the requested outcome

#### Scenario: Legacy compatibility expires safely
<!-- verification-id: OS-AEC-006 -->
- **WHEN** fewer than two minor releases or 90 days have elapsed since schema `1` shipped, or fleet telemetry still reports schema-`0` endpoints
- **THEN** the system continues to accept schema `0`

#### Scenario: Legacy compatibility is removed
<!-- verification-id: OS-AEC-007 -->
- **WHEN** both minimum support periods have elapsed, telemetry reports no schema-`0` endpoints, and a breaking release announced removal
- **THEN** schema `0` may be rejected with a migration diagnostic

### Requirement: Accepted fields are convergent
The system SHALL check and apply every accepted managed field. An omitted optional field SHALL mean unmanaged unless the field's specification declares a default.

#### Scenario: Managed metadata drifts
<!-- verification-id: OS-AEC-008 -->
- **WHEN** a resource's content is correct but one accepted managed metadata field differs
- **THEN** Check reports drift and Apply repairs that metadata field

#### Scenario: Field is unavailable in a provider
<!-- verification-id: OS-AEC-009 -->
- **WHEN** a resource requests a field that the selected provider cannot check and apply
- **THEN** the artifact is rejected as an unsupported field/provider combination rather than silently ignoring the field

### Requirement: Structured check outcomes
Each applicator SHALL return exactly one check status from `compliant`, `drifted`, `unsupported`, `check_failed`, or `deferred`, together with a stable reason code and redacted desired and observed summaries when applicable.

#### Scenario: Probe command fails
<!-- verification-id: OS-AEC-010 -->
- **WHEN** an applicator cannot observe state because its probe fails
- **THEN** the result is `check_failed` and is not counted as either compliance or drift

#### Scenario: Provider is unavailable
<!-- verification-id: OS-AEC-011 -->
- **WHEN** no compatible provider is present on the endpoint
- **THEN** the result is `unsupported` and identifies the required capability without entering an apply loop

### Requirement: Apply eligibility follows check status
The engine SHALL apply only resources whose status is `drifted`, whose dependencies are satisfied, whose policy permits remediation, and whose safety preconditions pass.

#### Scenario: Unsupported resource is not applied
<!-- verification-id: OS-AEC-012 -->
- **WHEN** Check returns `unsupported`
- **THEN** the engine does not invoke Apply and reports the unsupported result

#### Scenario: Report policy observes drift
<!-- verification-id: OS-AEC-013 -->
- **WHEN** Check returns `drifted` under report-only policy
- **THEN** the resource remains unchanged and the report records remediation as skipped by policy

### Requirement: Stable identity and dependency behavior
Every resource SHALL have a stable `<configuration>/<resource-name>` address, and names SHALL be unique across all resource kinds within one configuration. Dependencies SHALL reference stable addresses and SHALL prevent dependent application after failure, unsupported state, or deferral.

#### Scenario: Cross-kind duplicate name
<!-- verification-id: OS-AEC-014 -->
- **WHEN** two resources of different kinds share a name in one configuration
- **THEN** validation fails because their stable address would be ambiguous

#### Scenario: Dependency apply fails
<!-- verification-id: OS-AEC-015 -->
- **WHEN** a dependency fails during Apply
- **THEN** dependent resources are reported blocked and are not applied

### Requirement: Provider capability negotiation
Every modern agent SHALL report a bounded, versioned endpoint capability document on every authenticated Sync containing supported artifact schema versions, stable capability IDs and contract revisions, normalized provider facts, agent version metadata, and a digest of the canonical document body. The server SHALL validate identity, bounds, grammar, uniqueness, internal consistency, and digest; record its own receive time; and persist the latest valid document for readiness and reporting. Artifact selection SHALL use only a valid document in the current authenticated Sync. Agent version SHALL NOT by itself prove runtime provider support for a modern agent. Statically impossible target/provider combinations SHALL fail author-time validation; endpoint-local mismatches SHALL produce `unsupported`.

#### Scenario: RPM provider targeted before support
<!-- verification-id: OS-AEC-016 -->
- **WHEN** configuration targets an RPM-family package provider not advertised by the target agent matrix
- **THEN** configuration validation rejects the release

#### Scenario: Expected backend binary is missing
<!-- verification-id: OS-AEC-017 -->
- **WHEN** the artifact is valid for the endpoint facts but the required backend is absent locally
- **THEN** Check returns `unsupported` with the backend capability reason

#### Scenario: Legacy agent reports no capability document
<!-- verification-id: OS-AEC-018 -->
- **WHEN** a known legacy agent version syncs without a capability document
- **THEN** the server assigns only the conservative schema-0 capability profile mapped to that version

#### Scenario: Unknown agent reports no capability document
<!-- verification-id: OS-AEC-019 -->
- **WHEN** an unknown agent version syncs without a capability document
- **THEN** the server assumes only the minimal legacy baseline and fails closed for newer capabilities

#### Scenario: Modern agent omits current capability evidence
<!-- verification-id: OS-AEC-020 -->
- **WHEN** an agent version that implements capability reporting omits or sends an invalid document on Sync
- **THEN** the server keeps its last active artifact and reports capability-blocked instead of selecting from persisted evidence

#### Scenario: Endpoint reconnects after being offline
<!-- verification-id: OS-AEC-021 -->
- **WHEN** an endpoint's persisted capability document is old and the endpoint reconnects with a valid current document
- **THEN** selection uses the current document without applying an arbitrary wall-clock freshness threshold

#### Scenario: Capability document digest is invalid
<!-- verification-id: OS-AEC-088 -->
- **WHEN** the server recomputes a canonical capability-document digest different from the submitted digest
- **THEN** it rejects the current document, retains the endpoint's active artifact, and does not select using persisted evidence

#### Scenario: Capability document exceeds a bound
<!-- verification-id: OS-AEC-089 -->
- **WHEN** a Sync document exceeds its byte, entry-count, identifier, or revision bound
- **THEN** the server returns a bounded validation result without persisting or using the document

### Requirement: Artifact delivery is capability-compatible
Composition SHALL calculate an explicit versioned requirement set for each bounded artifact variant and SHALL cache only canonical schema 1 and, during migration, a schema-0 variant when conversion is behaviorally lossless. On authenticated Sync, the server SHALL serve the highest target variant satisfied by the current capability document and SHALL NOT omit resources or fields to manufacture compatibility. Offered state SHALL not become active until the endpoint acknowledges successful processing of the exact digest.

#### Scenario: Legacy variant is lossless
<!-- verification-id: OS-AEC-022 -->
- **WHEN** the current desired state can be represented in schema 0 without losing behavior and an endpoint supports only schema 0
- **THEN** the server serves the schema-0 compatibility variant for the current Release ref

#### Scenario: No compatible current artifact exists
<!-- verification-id: OS-AEC-023 -->
- **WHEN** the current Release ref requires capabilities absent from an existing endpoint
- **THEN** the global Release ref advances, the endpoint continues checking its last successfully processed artifact, and Sync reports `capability_blocked` with the unavailable Release ref and missing capabilities

#### Scenario: New endpoint has no compatible artifact
<!-- verification-id: OS-AEC-024 -->
- **WHEN** a newly enrolled endpoint has no prior artifact and cannot satisfy any current variant
- **THEN** it remains explicitly unmanaged and `capability_blocked` rather than receiving partial desired state

#### Scenario: Compatible agent upgrade is available
<!-- verification-id: OS-AEC-025 -->
- **WHEN** an endpoint is capability-blocked and an approved agent version satisfies the missing contract
- **THEN** the server may include that agent-upgrade instruction without claiming the current artifact is active

#### Scenario: Endpoint-specific variant would require field removal
<!-- verification-id: OS-AEC-090 -->
- **WHEN** no bounded variant is compatible unless one or more desired resources or fields are omitted
- **THEN** composition creates no endpoint-specific partial variant and delivery remains capability-blocked

#### Scenario: Offered artifact is not acknowledged
<!-- verification-id: OS-AEC-091 -->
- **WHEN** the server offers a compatible target artifact but the endpoint does not acknowledge successful processing of its digest
- **THEN** endpoint active Release and active artifact digest remain unchanged

### Requirement: Artifact release state is observable
Operator reporting SHALL distinguish the global or endpoint-override target Release ref, the endpoint's active processed Release ref and artifact digest, any currently offered Release ref, active artifact schema version, current capability digest, persisted capability receive time, and any capability-blocked target Release with exact bounded missing requirements. Telemetry SHALL remain attributed to the active artifact digest.

#### Scenario: Endpoint remains on an older artifact
<!-- verification-id: OS-AEC-026 -->
- **WHEN** the global Release ref advances but an endpoint is capability-blocked
- **THEN** state reporting shows both refs and does not attribute checks against the old active artifact to the newer Release ref

#### Scenario: Blocked endpoint submits pending telemetry
<!-- verification-id: OS-AEC-092 -->
- **WHEN** a capability-blocked endpoint submits bounded telemetry for its active older artifact
- **THEN** the server persists it under that active digest without treating the endpoint as having processed the blocked target Release

### Requirement: Explicit ownership and lifecycle
Each resource kind SHALL declare whether it owns one named object, one Remotr-owned fragment, or an authoritative set. Absence or garbage collection SHALL occur only when explicitly requested by lifecycle or authoritative ownership.

#### Scenario: Resource omitted from later artifact
<!-- verification-id: OS-AEC-027 -->
- **WHEN** a previously managed resource is omitted and no authoritative-set contract covers it
- **THEN** the endpoint state is left unchanged

#### Scenario: Authoritative set removes extra managed member
<!-- verification-id: OS-AEC-028 -->
- **WHEN** an authoritative set contains fewer members than the prior desired set
- **THEN** Apply removes only members inside that set's declared ownership boundary

### Requirement: Risk classes and preflight
Resource kinds SHALL declare their risk class. Connectivity, access, boot, destructive, and sensitive changes SHALL require kind-specific preflight and SHALL default to non-enforcing behavior where their capability specification requires it.

#### Scenario: High-risk preflight fails
<!-- verification-id: OS-AEC-029 -->
- **WHEN** a high-risk resource fails syntax, recovery, connectivity, capacity, or other required preflight
- **THEN** Apply is blocked without activating partial state and the structured reason is reported

### Requirement: High-risk changes have a reviewable request
When a merged Release ref changes related high-risk resources, the server SHALL create a Change request containing the exact Release ref, artifact and resource hashes, risk classes, frozen evaluated endpoint set, compatibility, predicted effects, rollback capability, and per-endpoint preflight evidence. High-risk Apply SHALL remain blocked until the request has matching authorization.

#### Scenario: Normal resource accompanies pending high-risk resource
<!-- verification-id: OS-AEC-030 -->
- **WHEN** a Release ref contains both normal and unauthorized high-risk drift
- **THEN** normal resources may converge while high-risk resources and their dependents remain non-enforcing

#### Scenario: Endpoint joins after rollout target freeze
<!-- verification-id: OS-AEC-031 -->
- **WHEN** a new endpoint joins after a Rollout authorization's target set was frozen
- **THEN** it is not added to that rollout authorization

### Requirement: Change-control state is durable across server restarts
The server SHALL persist Change requests, approvals, Rollout and Fleet baseline authorizations, Approval policy and warnings, target outcomes and audit history, automatic-promotion policy, Execution leases and attempt accounting, progress, and Break-glass authorizations in the server registry. Each mutation SHALL commit atomically before success is reported. Startup SHALL restore and validate the persisted state before serving Admin API or authenticated Sync traffic and SHALL fail closed rather than replace unreadable state with an empty registry.

#### Scenario: Server restarts during an authorized rollout
<!-- verification-id: OS-AEC-076 -->
- **WHEN** the server restarts after a Change request has collected approvals and received a Rollout authorization
- **THEN** the Admin API returns the same frozen plan, approvals, authorization state, outcomes, and audit history, and an eligible endpoint can continue the rollout without reauthorization

#### Scenario: Server restarts with an active Execution lease
<!-- verification-id: OS-AEC-077 -->
- **WHEN** an unexpired lease occupies the approved concurrency slot and the server restarts
- **THEN** authenticated Sync does not issue a duplicate or over-limit lease, the original attempt count remains authoritative, and the slot is released only by completion or ordinary expiry

#### Scenario: Change-control persistence fails
<!-- verification-id: OS-AEC-078 -->
- **WHEN** an Admin API or authenticated Sync mutation cannot commit to the server registry
- **THEN** the operation reports failure, leaves the prior observable state unchanged, and delivers no unpersisted authorization or Execution lease

#### Scenario: Persisted Change-control state is unreadable
<!-- verification-id: OS-AEC-079 -->
- **WHEN** server startup cannot load or validate stored Change-control state
- **THEN** startup fails closed with a safe diagnostic and does not serve an empty replacement registry

### Requirement: Authorization grouping is explicit and bounded
Resources MAY declare `authorizationGroup` to coordinate review and rollout of related high-risk state. A Change request SHALL NOT span Fleets or endpoint overrides. Without an explicit group, the server SHALL group high-risk resources by dependency-connected component, including shared activation edges; independent components SHALL produce separate requests. A mixed-risk group SHALL inherit its strictest authorization and rollout policy.

#### Scenario: Explicit network transition group
<!-- verification-id: OS-AEC-032 -->
- **WHEN** DNS, route, firewall, and network-profile resources share one authorization group in a Fleet
- **THEN** the server creates one Change request containing their combined plan and strictest applicable risk policy

#### Scenario: Unrelated high-risk change shares a Release ref
<!-- verification-id: OS-AEC-033 -->
- **WHEN** an independent sudo-policy resource changes in the same Release ref as a grouped network transition
- **THEN** the server creates a separate Change request for the sudo-policy component

#### Scenario: Same group name appears in two Fleets
<!-- verification-id: OS-AEC-034 -->
- **WHEN** resources in two Fleets use the same authorization group name
- **THEN** each Fleet receives a separate Change request and authorization history

#### Scenario: Normal prerequisite is included in the plan
<!-- verification-id: OS-AEC-035 -->
- **WHEN** a high-risk resource depends on a normal package or file resource
- **THEN** the prerequisite appears in the Change request plan but retains its normal risk class

### Requirement: Rollout authorization is temporary and exact
A Rollout authorization SHALL bind the exact desired-state hashes to a frozen endpoint set, maintenance window, expiry, attempt limit, concurrency policy, approving operator, and justification.

#### Scenario: Desired state changes after authorization
<!-- verification-id: OS-AEC-036 -->
- **WHEN** any authorized high-risk resource field changes
- **THEN** its desired-state hash no longer matches and a new Change request is required

### Requirement: Approval thresholds are configurable by risk
The server registry SHALL define global Approval policy with optional Fleet and risk-class overrides. By default, connectivity, access/privilege, boot policy, coordinated reboot, and their baseline promotion SHALL require one approver, while destructive storage SHALL require two. Approvals SHALL count distinct operator identities with appropriate RBAC permission rather than distinct credentials.

#### Scenario: Operator has two credentials
<!-- verification-id: OS-AEC-037 -->
- **WHEN** the same operator identity approves through two valid certificates
- **THEN** those approvals count once toward the threshold

#### Scenario: Destructive change has one approval
<!-- verification-id: OS-AEC-038 -->
- **WHEN** a destructive-storage Change request has only one approval under default policy
- **THEN** authorization remains pending at one of two approvals

#### Scenario: Small installation lowers destructive threshold
<!-- verification-id: OS-AEC-039 -->
- **WHEN** a single-operator installation explicitly configures one destructive approver
- **THEN** the server permits the policy and continuously exposes an audit warning

### Requirement: Approval is bound to an unchanged plan
An approval SHALL bind resource hashes, target scope, authorization lifetime, maintenance constraints, and rollout-control bounds. Changing any bound field SHALL invalidate collected approvals.

#### Scenario: Concurrency increases after approval
<!-- verification-id: OS-AEC-040 -->
- **WHEN** an operator raises maximum concurrency beyond the approved bound
- **THEN** prior approvals are invalidated and the authorization returns to approval-pending

#### Scenario: Baseline promotion was approved initially
<!-- verification-id: OS-AEC-041 -->
- **WHEN** the original authorization explicitly includes baseline-on-success and satisfies its risk policy
- **THEN** successful verified rollout may promote the exact hashes without a second approval round

#### Scenario: Baseline promotion is requested later
<!-- verification-id: OS-AEC-042 -->
- **WHEN** baseline promotion was not included in the rollout authorization
- **THEN** the promotion must satisfy the current Approval policy with fresh approvals

### Requirement: Proven state can become a fleet baseline
A baseline-eligible high-risk resource SHALL be promotable to a Fleet baseline authorization only after successful verified rollout and explicit authorization. The baseline SHALL bind fleet, resource address, desired-state hash, risk class, and provider constraints. A future fleet member SHALL apply it automatically only after passing current capability and safety preflight.

#### Scenario: New endpoint matches an approved baseline
<!-- verification-id: OS-AEC-043 -->
- **WHEN** a later-enrolled endpoint belongs to the fleet, resolves the same resource hash, and passes local preflight
- **THEN** it may converge without replaying the historical Rollout authorization

#### Scenario: New endpoint fails baseline preflight
<!-- verification-id: OS-AEC-044 -->
- **WHEN** a later-enrolled endpoint resolves an approved baseline but fails provider or safety preflight
- **THEN** only that endpoint is blocked and the baseline authorization remains valid for other compatible endpoints

### Requirement: Authorization validity and execution windows are separate
Rollout authorization SHALL have configurable validity with a 30-day default and MAY contain recurring execution windows. A frozen target SHALL execute only while authorization is valid, an execution window is open, and its current preflight passes.

#### Scenario: Offline endpoint reconnects during a later window
<!-- verification-id: OS-AEC-045 -->
- **WHEN** a frozen target was offline during the first window but reconnects during a later window before authorization expiry
- **THEN** it may execute after a fresh successful preflight without new approval

#### Scenario: Endpoint reconnects after authorization expiry
<!-- verification-id: OS-AEC-046 -->
- **WHEN** a frozen target reconnects after Rollout authorization expires and no Fleet baseline authorization exists
- **THEN** it remains non-enforcing until a new authorization is approved

### Requirement: High-risk work requires an endpoint execution lease
The server SHALL issue a short-lived Execution lease only when the endpoint is in the frozen target set, current preflight passes, authorization remains valid, an Execution window is open, and a concurrency slot is available. The lease SHALL bind Change request, endpoint, resource hashes, attempt, and expiry and SHALL be delivered during authenticated Sync. The agent SHALL NOT begin high-risk work from cached authorization alone.

#### Scenario: Concurrency limit is reached
<!-- verification-id: OS-AEC-047 -->
- **WHEN** all approved concurrency slots are leased
- **THEN** another ready endpoint remains scheduled until a slot becomes available

#### Scenario: Change request is paused
<!-- verification-id: OS-AEC-048 -->
- **WHEN** an operator pauses a Change request
- **THEN** the server issues no new leases while already applying endpoints continue to verification or rollback

#### Scenario: Cached lease has expired
<!-- verification-id: OS-AEC-049 -->
- **WHEN** an endpoint has not begun work before its lease expiry
- **THEN** it discards the lease and requests a new one on a later Sync

### Requirement: Acknowledgement is risk-specific
Each high-risk provider SHALL define preparation, verification, acknowledgement, timeout, and rollback behavior appropriate to its risk. Progress SHALL report lease-issued, prepared, applying, verifying, acknowledged, rolled-back, failed, or acknowledgement-timeout.

#### Scenario: Network transition reconnects
<!-- verification-id: OS-AEC-050 -->
- **WHEN** a network or firewall change completes a new authenticated Sync over the changed path before timeout
- **THEN** the server acknowledges the attempt and the endpoint cancels its armed rollback

#### Scenario: Network transition loses control path
<!-- verification-id: OS-AEC-051 -->
- **WHEN** no authenticated Sync acknowledgement arrives before timeout
- **THEN** the provider checkpoint or independent local watchdog restores prior network state and reports rollback outcome

#### Scenario: Access policy passes local validation
<!-- verification-id: OS-AEC-052 -->
- **WHEN** SSH, sudo, or PAM effective validation passes and Remotr Sync remains available
- **THEN** the endpoint may report technical verification while clearly not claiming that a human login was tested

#### Scenario: Reboot reconnects with a new boot ID
<!-- verification-id: OS-AEC-053 -->
- **WHEN** a leased reboot acknowledges intent and later reconnects with a different boot ID
- **THEN** the attempt is acknowledged as successful

#### Scenario: Irreversible storage succeeds
<!-- verification-id: OS-AEC-054 -->
- **WHEN** an authorized destructive operation satisfies exact device preconditions and postconditions
- **THEN** it reports verified success and rollback capability `none` rather than inventing a revert path

### Requirement: Emergency authorization is explicit and bounded
Emergency operation SHALL use a dedicated Break-glass authorization requiring risk-specific RBAC permission, exact resource hashes and targets, justification, external incident/change reference, bounded validity, and attempt count. It MAY bypass normal approval count, Execution windows, and concurrency but SHALL NOT bypass schema/provider validation, redaction, current preflight, required rollback, or destructive-storage identity and irreversible-operation approval.

#### Scenario: Endpoint break glass uses defaults
<!-- verification-id: OS-AEC-055 -->
- **WHEN** an authorized operator creates break glass without wider bounds
- **THEN** it applies to one endpoint, one attempt, and at most 60 minutes

#### Scenario: Fleet-wide break glass is requested
<!-- verification-id: OS-AEC-056 -->
- **WHEN** break glass targets an entire Fleet
- **THEN** explicit fleet scope and a second distinct operator are required

#### Scenario: Break glass targets destructive storage
<!-- verification-id: OS-AEC-057 -->
- **WHEN** an operator attempts to bypass stable device identity or irreversible-operation approval
- **THEN** the request is rejected even with break-glass permission

#### Scenario: Break glass lifecycle is audited
<!-- verification-id: OS-AEC-058 -->
- **WHEN** break glass is created, used, expires, or is revoked
- **THEN** the server emits a prominent audit event suitable for SIEM alerting

#### Scenario: Server is unreachable
<!-- verification-id: OS-AEC-059 -->
- **WHEN** an operator cannot reach the Remotr server
- **THEN** break glass is unavailable and documentation directs the operator to out-of-band recovery

### Requirement: Rollout evidence accounts for every frozen target
Change request reporting SHALL classify every frozen target as verified successful, failed or rolled back, capability or preflight blocked, or not seen during authorization.

#### Scenario: Laptop remains offline throughout rollout
<!-- verification-id: OS-AEC-060 -->
- **WHEN** a frozen target never Syncs during authorization validity
- **THEN** the completed rollout reports it as not seen rather than successful or failed

### Requirement: Baseline promotion is manual by default
Manual baseline promotion SHALL be the default. Offline, blocked, and not-seen targets SHALL NOT prevent manual promotion, but the operator SHALL see and explicitly acknowledge exceptions under the applicable Approval policy.

#### Scenario: Operator promotes with offline exceptions
<!-- verification-id: OS-AEC-061 -->
- **WHEN** verified endpoints succeeded, no unresolved failure is hidden, and the operator acknowledges not-seen endpoints with required approvals
- **THEN** the exact resource hashes become the Fleet baseline and the remaining endpoints must pass current preflight before applying

### Requirement: Automatic baseline promotion is policy-defined
Automatic baseline-on-success SHALL be unavailable unless Fleet Approval policy explicitly defines canary stages, minimum successful evidence, and maximum failures. Any unresolved failure or rollback SHALL block automatic promotion; the system SHALL NOT use a hidden universal percentage threshold.

#### Scenario: Automatic promotion has unresolved rollback
<!-- verification-id: OS-AEC-062 -->
- **WHEN** the configured success threshold is otherwise met but any target has an unresolved rollback
- **THEN** automatic baseline promotion remains blocked

### Requirement: Event and destructive authorization do not become baselines
Reboot events, destructive storage grants, and emergency overrides SHALL NOT be promotable to Fleet baseline authorization. Destructive storage SHALL remain bound to endpoint-specific stable device identity.

#### Scenario: New endpoint joins after a reboot rollout
<!-- verification-id: OS-AEC-063 -->
- **WHEN** a fleet reboot completed before the endpoint enrolled
- **THEN** the endpoint does not replay that reboot from baseline history

### Requirement: Existing fleets can adopt one reviewed baseline
The system SHALL support aggregating the currently resolved high-risk state of an existing fleet into one preflighted baseline-adoption request without creating a separate historical request for every resource.

#### Scenario: Fleet enables authorization feature
<!-- verification-id: OS-AEC-064 -->
- **WHEN** an existing fleet already contains many unchanged high-risk resources
- **THEN** an operator can review and authorize their exact current hashes as one baseline adoption

### Requirement: Exclusive operation locks
The agent SHALL serialize resources that share an exclusive lock domain and SHALL honor provider-native locks with bounded waits.

#### Scenario: Package database is busy
<!-- verification-id: OS-AEC-065 -->
- **WHEN** the native package database lock cannot be acquired before the configured timeout
- **THEN** Apply returns a retryable lock failure without starting a competing transaction

### Requirement: Structured activation outcomes
Applicators SHALL return activation needs as structured signals, and the engine SHALL order and coalesce compatible reload, restart, daemon-reload, logout, next-boot, and reboot-required signals.

#### Scenario: Multiple unit fragments change
<!-- verification-id: OS-AEC-066 -->
- **WHEN** multiple resources require the same daemon reload in one successful run
- **THEN** the engine performs one correctly ordered daemon reload before dependent service activation

### Requirement: Honest rollback reporting
Each resource SHALL advertise rollback as `transactional`, `best_effort`, or `none`. Apply failures SHALL preserve the original error and report rollback outcome separately.

#### Scenario: Apply and rollback both fail
<!-- verification-id: OS-AEC-067 -->
- **WHEN** Apply fails and its best-effort rollback also fails
- **THEN** reporting retains both failures and does not replace the apply error with the rollback error

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

### Requirement: Secret rollback prefers version references
Server-managed secret rollback SHALL retain a prior Secret-version reference rather than generic secret bytes whenever online resolution can satisfy recovery. Disconnected recovery MAY retain encrypted prior bytes only until acknowledgement, with the sensitive-payload retention bound. A referenced version SHALL NOT be deleted until recovery expires or an authorized operator abandons it.

#### Scenario: Wi-Fi transition is awaiting acknowledgement
<!-- verification-id: OS-AEC-071 -->
- **WHEN** rollback must operate after loss of server connectivity
- **THEN** the endpoint retains encrypted prior credential bytes only until acknowledgement, rollback, or the 24-hour maximum

#### Scenario: Operator deletes a referenced prior version
<!-- verification-id: OS-AEC-072 -->
- **WHEN** a Secret version remains referenced by armed or retained rollback
- **THEN** deletion is blocked unless an authorized operator explicitly abandons that recovery path

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

### Requirement: Fleet state distinguishes outcomes
Endpoint and fleet reports SHALL separately aggregate compliant, drifted, unsupported, check-failed, deferred, apply-failed, and no-report states.

#### Scenario: Fleet contains mixed outcomes
<!-- verification-id: OS-AEC-075 -->
- **WHEN** a fleet has endpoints in drift, unsupported, and no-report states
- **THEN** each endpoint and each fleet summary bucket reflects its actual state without labeling unsupported endpoints as drifted

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
