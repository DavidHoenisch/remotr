## MODIFIED Requirements

### Requirement: Endpoint inventory uses canonical compliance state
The Endpoint inventory SHALL show Endpoint ID, Fleet, current State report status, independent target-delivery status, check-in freshness, reported and desired agent versions, active and target Release refs, selected Labels, and last evidence time. Canonical compliance SHALL be derived from the current State report rather than historical summary fields or capability delivery state. Delivery SHALL be classified independently as current, offered, capability-blocked, unmanaged, or not reported.

<!-- verification-id: OS-DFV-006 -->
#### Scenario: Endpoint statuses preserve Admin API distinctions
- **WHEN** State reports contain compliant, drifted, unsupported, check-failed, deferred, apply-failed, and no-report statuses
- **THEN** the table renders those statuses as distinct text labels and does not collapse them into a single healthy or unhealthy boolean

<!-- verification-id: OS-DFV-007 -->
#### Scenario: Endpoint without a State report is not declared healthy
- **WHEN** an enrolled Endpoint has check-in metadata but no State report
- **THEN** the inventory labels its compliance as not reported and shows the independent check-in freshness

<!-- verification-id: OS-DFV-008 -->
#### Scenario: Selected labels are deterministic
- **WHEN** Endpoints report different Label keys
- **THEN** the default Label columns use a deterministic configured key order and the operator can inspect all remaining key-value pairs in Endpoint detail

<!-- verification-id: OS-DFV-036 -->
#### Scenario: Compliant endpoint is blocked from the target release
- **WHEN** an Endpoint is compliant with its active artifact and capability-blocked from a newer target Release
- **THEN** inventory and detail show compliant and capability-blocked as separate statuses and identify the distinct active and target Release refs

<!-- verification-id: OS-DFV-037 -->
#### Scenario: Historical failure predates current compliance
- **WHEN** endpoint history contains an apply failure older than its newest compliant State report
- **THEN** inventory shows compliant while endpoint history may still expose the older failure as historical evidence
