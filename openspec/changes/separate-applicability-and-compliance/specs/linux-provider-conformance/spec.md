## ADDED Requirements

### Requirement: PopOS identity and provider qualification are exact
Endpoint facts SHALL report Pop!_OS as exact distribution `PopOS`/`popos` with Debian family lineage rather than reporting exact Debian or Ubuntu identity. A provider capability SHALL NOT be inherited from a Debian or Ubuntu qualification row merely because Pop!_OS shares their lineage.

#### Scenario: Pop!_OS row is not qualified
<!-- verification-id: OS-LPC-028 -->
- **WHEN** a Pop!_OS endpoint has a compatible package manager or init system but no complete passing provider row for its exact release and architecture
- **THEN** it advertises the normalized facts but not that provider capability and compatible delivery remains fail-closed
