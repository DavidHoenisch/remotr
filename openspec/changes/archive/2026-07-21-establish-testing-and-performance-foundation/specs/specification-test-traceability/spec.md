## ADDED Requirements

### Requirement: OpenSpec scenarios have immutable verification identity
Every OpenSpec scenario governing active or planned product behavior SHALL have one globally unique verification ID embedded adjacent to the scenario. The ID SHALL remain stable across wording and title changes and SHALL NOT be reused after removal.

#### Scenario: Scenario wording changes
<!-- verification-id: OS-STR-001 -->
- **WHEN** a contributor clarifies an existing scenario without changing its behavioral identity
- **THEN** the scenario retains its verification ID and existing evidence mappings remain valid

#### Scenario: Duplicate identifier is introduced
<!-- verification-id: OS-STR-002 -->
- **WHEN** two scenarios declare the same verification ID
- **THEN** repository validation fails before merge and identifies both sources

### Requirement: Traceability inventory is generated from canonical specs
The repository SHALL generate its scenario inventory from active and archived OpenSpec artifacts and SHALL compare that inventory with a machine-readable traceability manifest. A handwritten list SHALL NOT be the discovery mechanism.

#### Scenario: New scenario lacks a traceability entry
<!-- verification-id: OS-STR-003 -->
- **WHEN** an OpenSpec scenario is added with a valid verification ID but no manifest entry
- **THEN** traceability validation fails and reports the unclassified scenario

#### Scenario: Manifest entry has no source scenario
<!-- verification-id: OS-STR-004 -->
- **WHEN** the manifest contains an identifier that is absent from the canonical OpenSpec inventory
- **THEN** validation fails as an orphan unless an approved retained-history disposition exists

### Requirement: Every scenario has an explicit verification disposition
Each traceability entry SHALL identify its source, lifecycle status, verification class, test selectors and required environments when verified, or a reason when planned, deferred, removed, or not applicable. `planned` SHALL NOT count as verification evidence.

#### Scenario: Advertised behavior remains planned
<!-- verification-id: OS-STR-005 -->
- **WHEN** a capability is advertised while one of its governing scenarios is still classified `planned`
- **THEN** the advertisement gate fails because no implemented evidence exists

#### Scenario: Roadmap behavior is deferred
<!-- verification-id: OS-STR-006 -->
- **WHEN** a scenario belongs to an explicitly deferred provider or milestone
- **THEN** its entry records `deferred`, the governing decision, and no false passing evidence

### Requirement: Evidence may compose across layers
One automated test MAY verify multiple scenario IDs, and one scenario MAY require multiple evidence selectors. The manifest SHALL distinguish unit, contract, acceptance, container, VM, fuzz, mutation, performance, and manual evidence.

#### Scenario: Safety workflow requires multiple environments
<!-- verification-id: OS-STR-007 -->
- **WHEN** a connectivity rollback scenario requires an in-process authorization test and a VM recovery test
- **THEN** both selectors and their environment requirements are recorded against the same verification ID

### Requirement: Executable specifications are selected by behavior
Godog features SHALL be limited to cross-component, operator-visible, or safety-critical workflows that benefit from domain-readable executable examples. Lower-level permutations SHALL use the cheapest trustworthy Go test layer.

#### Scenario: Parser requirement has many malformed inputs
<!-- verification-id: OS-STR-008 -->
- **WHEN** a parser scenario can be exhausted through table-driven and fuzz tests without crossing a product boundary
- **THEN** traceability maps it to those tests without requiring a duplicate Godog scenario

#### Scenario: High-risk rollout crosses components
<!-- verification-id: OS-STR-009 -->
- **WHEN** authorization, Sync delivery, endpoint execution, acknowledgement, and operator reporting form one workflow
- **THEN** the scenario is eligible for a tagged Godog acceptance feature

### Requirement: Godog scenarios link to OpenSpec identity
Every committed Godog scenario SHALL carry at least one verification-ID tag, and validation SHALL reject missing, unknown, or deferred-only tags.

#### Scenario: Acceptance feature loses its specification link
<!-- verification-id: OS-STR-010 -->
- **WHEN** a `.feature` scenario has no valid OpenSpec verification tag
- **THEN** the acceptance traceability check fails before the feature can merge

### Requirement: Behavioral changes update specification and evidence together
A pull request that changes behavior SHALL update the governing OpenSpec scenario or confirm its unchanged identity, update traceability, and add or revise evidence at the agreed seams. Tests SHALL NOT be removed or weakened without the corresponding specification disposition.

#### Scenario: Test is removed while requirement remains active
<!-- verification-id: OS-STR-011 -->
- **WHEN** a pull request deletes the only evidence selector for a verified active scenario
- **THEN** traceability validation fails until replacement evidence or an approved specification change is supplied
