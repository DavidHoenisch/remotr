## MODIFIED Requirements

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
