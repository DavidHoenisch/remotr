## ADDED Requirements

### Requirement: Compliance and target delivery are independent
Endpoint inventory and detail SHALL present canonical current State-report compliance independently from target-delivery readiness. Delivery SHALL be classified as current, offered, capability-blocked, unmanaged, or not reported using active, offered, target, blocked, and unmanaged evidence without changing compliance.

<!-- verification-id: OS-DFV-036 -->
#### Scenario: Compliant endpoint is blocked from the target release
- **WHEN** an Endpoint is compliant with its active artifact and capability-blocked from a newer target Release
- **THEN** inventory and detail show compliant and capability-blocked as separate statuses and identify the distinct active and target Release refs

<!-- verification-id: OS-DFV-037 -->
#### Scenario: Historical failure predates current compliance
- **WHEN** endpoint history contains an apply failure older than its newest compliant State report
- **THEN** inventory shows compliant while endpoint history may still expose the older failure as historical evidence
