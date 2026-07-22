## ADDED Requirements

### Requirement: Qualified Ubuntu Pro capabilities are wired into production delivery
Every released production agent SHALL derive Ubuntu Pro capability advertisement from the frozen checked-in passing qualification rows included in that release. A matching endpoint SHALL advertise the base resource capability and only the exact passing service, mode, variant, and disable-behavior provider capabilities. Composition SHALL place Ubuntu Pro requirements only on target variants eligible for exact Canonical Ubuntu identity, release, and architecture. Public authenticated Sync evidence SHALL prove that a qualified endpoint can receive and acknowledge the complete Ubuntu Pro artifact before the row is considered delivery-ready.

#### Scenario: Default generator runs on qualified Ubuntu 26.04
<!-- verification-id: OS-UPM-061 -->
- **WHEN** the production default capability generator evaluates matching exact Ubuntu 26.04 amd64 facts and the release contains passing base attachment and `esm-apps` rows
- **THEN** it advertises the Ubuntu Pro resource, service, and exact selected option capabilities without test-only injection and without advertising unqualified tuples

#### Scenario: Qualified endpoint syncs the Ubuntu Pro artifact
<!-- verification-id: OS-UPM-062 -->
- **WHEN** an authenticated Ubuntu 26.04 amd64 endpoint reports the exact passing capabilities required by its Ubuntu Pro target variant
- **THEN** Sync offers the complete canonical artifact and records it active only after successful exact-digest acknowledgment

#### Scenario: Arch branch shares the fleet artifact
<!-- verification-id: OS-UPM-063 -->
- **WHEN** the same canonical fleet artifact also contains an Arch-targeted Pacman configuration
- **THEN** the qualified Ubuntu endpoint's Ubuntu Pro target variant is not blocked by Pacman requirements and no Arch resource is removed from the artifact

#### Scenario: Ubuntu Pro row is absent from the production catalog
<!-- verification-id: OS-UPM-064 -->
- **WHEN** an Ubuntu Pro implementation or test fixture exists but its exact passing row is absent from the packaged production catalog
- **THEN** the endpoint does not advertise the capability and validation or delivery fails closed without resolving the enrollment token
