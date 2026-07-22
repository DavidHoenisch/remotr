## MODIFIED Requirements

### Requirement: Exact distribution identity is distinct from family compatibility
Endpoint facts and provider selection SHALL preserve an exact operating-system identity separately from compatibility lineage. Pop!_OS SHALL report exact distribution `PopOS`/`popos` and Debian family lineage rather than reporting exact Debian or Ubuntu identity. A provider scoped to Canonical Ubuntu SHALL require exact, consistent Ubuntu release and vendor evidence and SHALL NOT infer identity from `ID_LIKE`, package manager, init system, kernel, branding, or the presence of provider binaries. A provider capability SHALL NOT be inherited from a Debian or Ubuntu qualification row merely because Pop!_OS shares their lineage.

#### Scenario: Derivative is compatible with Debian family
<!-- verification-id: OS-LPC-011 -->
- **WHEN** Pop!_OS reports Debian or Ubuntu lineage and uses APT
- **THEN** its facts contain exact Pop!_OS identity and Debian family lineage while Ubuntu-only capabilities remain absent

#### Scenario: Exact and compatibility sources conflict
<!-- verification-id: OS-LPC-012 -->
- **WHEN** operating-system identity sources disagree or exact product identity is ambiguous
- **THEN** the endpoint does not advertise an exact-distribution provider and that provider performs no mutation

#### Scenario: Pop!_OS row is not qualified
<!-- verification-id: OS-LPC-028 -->
- **WHEN** a Pop!_OS endpoint has a compatible package manager or init system but no complete passing provider row for its exact release and architecture
- **THEN** it advertises the normalized facts but not that provider capability and compatible delivery remains fail-closed
