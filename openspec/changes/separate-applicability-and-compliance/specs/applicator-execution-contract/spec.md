## MODIFIED Requirements

### Requirement: Artifact delivery is capability-compatible
Composition SHALL calculate an explicit versioned requirement set for each bounded artifact variant and SHALL cache only canonical schema 1 and, during migration, a schema-0 variant when conversion is behaviorally lossless. Every variant SHALL have a canonical target predicate derived only from authored exact normalized distribution, release, architecture, and provider applicability; its requirement set SHALL include only matching configuration requirements plus portable requirements. Pop!_OS SHALL use the exact `popos` distribution predicate and SHALL NOT match exact `debian` or `ubuntu` predicates. Variants MAY share identical complete desired-state bytes and digest, but the server SHALL NOT omit resources or fields or create endpoint-identity-specific artifacts to manufacture compatibility. On authenticated Sync, the server SHALL first match current capability-document facts to the target predicate and then serve the highest matching variant whose requirements are satisfied. Offered state SHALL not become active until the endpoint acknowledges successful processing of the exact digest.

An explicitly requested agent upgrade SHALL be evaluated independently from artifact-provider compatibility. When its approved release, binary platform, architecture, integrity, and authorization constraints pass, the server SHALL include it for a capability-blocked endpoint without claiming that the target agent version proves the missing runtime provider capabilities or that the blocked artifact is active. After upgrade, selection SHALL use the endpoint's newly reported valid capability document and MAY remain capability-blocked.

#### Scenario: Legacy variant is lossless
<!-- verification-id: OS-AEC-111 -->
- **WHEN** the current desired state can be represented in schema 0 without losing behavior and an endpoint supports only schema 0
- **THEN** the server serves the schema-0 compatibility variant for the current Release ref

#### Scenario: No compatible current artifact exists
<!-- verification-id: OS-AEC-112 -->
- **WHEN** the current Release ref's matching target variant requires capabilities absent from an existing endpoint
- **THEN** the global Release ref advances, the endpoint continues checking its last successfully processed artifact, and Sync reports `capability_blocked` with the unavailable Release ref and exact bounded missing capabilities

#### Scenario: New endpoint has no compatible artifact
<!-- verification-id: OS-AEC-113 -->
- **WHEN** a newly enrolled endpoint has no prior artifact and cannot satisfy any matching current variant
- **THEN** it remains explicitly unmanaged and `capability_blocked` rather than receiving partial desired state

#### Scenario: Compatible agent upgrade is available
<!-- verification-id: OS-AEC-114 -->
- **WHEN** an endpoint is capability-blocked and an explicitly requested approved agent release is eligible for the endpoint's binary platform and architecture
- **THEN** the server includes that agent-upgrade instruction without claiming the current artifact is active or that the target version satisfies any runtime provider requirement

#### Scenario: Endpoint-specific variant would require field removal
<!-- verification-id: OS-AEC-115 -->
- **WHEN** no bounded target variant is compatible unless one or more desired resources or fields are omitted
- **THEN** composition creates no endpoint-specific partial variant and delivery remains capability-blocked

#### Scenario: Offered artifact is not acknowledged
<!-- verification-id: OS-AEC-116 -->
- **WHEN** the server offers a compatible target artifact but the endpoint does not acknowledge successful processing of the exact digest
- **THEN** endpoint active Release and active artifact digest remain unchanged

#### Scenario: Mixed fleet contains Ubuntu and Arch package branches
<!-- verification-id: OS-AEC-108 -->
- **WHEN** one canonical fleet artifact contains an APT branch targeting Ubuntu and a Pacman branch targeting Arch
- **THEN** an Ubuntu endpoint's matching variant requires APT but not Pacman, while an Arch endpoint's matching variant requires Pacman but not APT, without removing either branch from the canonical desired state

#### Scenario: Upgraded endpoint still lacks a provider
<!-- verification-id: OS-AEC-109 -->
- **WHEN** a blocked endpoint installs the requested agent release and its next valid capability document still lacks an artifact requirement
- **THEN** the server retains the prior active artifact, reports the actual remaining requirement, and does not infer support from the installed agent version

#### Scenario: Legacy agent ignores capability-blocked metadata
<!-- verification-id: OS-AEC-110 -->
- **WHEN** a supported legacy agent can decode `agentUpgrade` but ignores the newer capability-blocked response field
- **THEN** the response remains backward compatible and the eligible upgrade instruction is still actionable without an artifact payload

#### Scenario: Pop!_OS does not inherit Ubuntu or Debian targets
<!-- verification-id: OS-AEC-117 -->
- **WHEN** one canonical Fleet artifact contains exact Ubuntu, Debian, and PopOS target branches and an endpoint reports exact Pop!_OS identity
- **THEN** its matching variant includes PopOS requirements plus portable requirements and excludes exact Ubuntu and Debian requirements without removing any branch from canonical desired state

## ADDED Requirements

### Requirement: State report classification follows current evidence
The current State report SHALL determine compliance status. A persisted apply-failure summary SHALL override that report only when it belongs to the same Release and is not older than the report; older or different-Release failures SHALL remain historical endpoint evidence and SHALL NOT change current compliance.

#### Scenario: Later compliant report supersedes apply failure
<!-- verification-id: OS-AEC-118 -->
- **WHEN** an Endpoint reports compliant State evidence after an apply failure for the same or an older Release
- **THEN** the State-report API returns compliant without a current apply failure while endpoint history retains the failure record

#### Scenario: Current failure follows the latest report
<!-- verification-id: OS-AEC-119 -->
- **WHEN** an apply failure for the same Release is reported after the latest State report
- **THEN** the State-report API classifies the Endpoint as apply-failed until newer State evidence supersedes it
