## ADDED Requirements

### Requirement: Exact derivative targets remain distinct
Artifact variants SHALL derive applicability from exact normalized distribution predicates plus portable requirements. Pop!_OS SHALL use the exact `popos` predicate and SHALL NOT match exact `debian` or `ubuntu` predicates. The server SHALL retain complete canonical desired-state bytes and SHALL NOT remove sibling target branches to manufacture compatibility.

#### Scenario: Pop!_OS does not inherit Ubuntu or Debian targets
<!-- verification-id: OS-AEC-117 -->
- **WHEN** one canonical Fleet artifact contains exact Ubuntu, Debian, and PopOS target branches and an endpoint reports exact Pop!_OS identity
- **THEN** its matching variant includes PopOS requirements plus portable requirements and excludes exact Ubuntu and Debian requirements without removing any branch from canonical desired state

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
