## ADDED Requirements

### Requirement: Release validation is consistent across public ingestion paths
The public configuration CLI and server Git-sync ingestion SHALL use the same target normalization, provider-catalog lookup, requirement derivation, ambiguity, and bounded-variant validation rules. Given the same repository and frozen capability catalog, they SHALL return the same acceptance classification and stable diagnostic identity. A rejected Git sync SHALL preserve the prior Release ref, return a bounded actionable diagnostic through the Admin API, and record the correlated internal cause in server logs without exposing secret material or raw sensitive desired state.

#### Scenario: Target architecture is missing for a provider-specific branch
<!-- verification-id: OS-AEC-104 -->
- **WHEN** a provider-specific configuration cannot be assigned an unambiguous supported distribution, release, and architecture tuple
- **THEN** `config validate` rejects its source file and resource address before Git sync with the same diagnostic identity the server validator would return

#### Scenario: Git sync encounters an unsupported target row
<!-- verification-id: OS-AEC-105 -->
- **WHEN** Git sync validates a repository that selects a provider row absent from the frozen release capability catalog
- **THEN** the server does not advance the Release ref, returns the safe file/resource/target/provider diagnostic, and logs the correlated internal cause

#### Scenario: Validation failure contains secret-bearing configuration
<!-- verification-id: OS-AEC-106 -->
- **WHEN** a rejected resource contains a secret reference, authenticated URL, or another classified value
- **THEN** CLI output, Admin API output, and server logs identify the safe field and failure class without including resolved bytes, URL credentials, environment contents, or raw sensitive fragments

#### Scenario: CLI and server validate the same corpus
<!-- verification-id: OS-AEC-107 -->
- **WHEN** valid, malformed, unsupported, ambiguous, and boundary repositories are exercised through both public validation seams against the same catalog
- **THEN** accept/reject results and stable diagnostic identities agree for every case

## MODIFIED Requirements

### Requirement: Artifact delivery is capability-compatible
Composition SHALL calculate an explicit versioned requirement set for each bounded artifact variant and SHALL cache only canonical schema 1 and, during migration, a schema-0 variant when conversion is behaviorally lossless. Every variant SHALL have a canonical target predicate derived only from authored normalized distribution, release, architecture, and provider applicability; its requirement set SHALL include only matching configuration requirements plus portable requirements. Variants MAY share identical complete desired-state bytes and digest, but the server SHALL NOT omit resources or fields or create endpoint-identity-specific artifacts to manufacture compatibility. On authenticated Sync, the server SHALL first match current capability-document facts to the target predicate and then serve the highest matching variant whose requirements are satisfied. Offered state SHALL not become active until the endpoint acknowledges successful processing of the exact digest.

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
- **WHEN** the server offers a compatible target artifact but the endpoint does not acknowledge successful processing of its digest
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
