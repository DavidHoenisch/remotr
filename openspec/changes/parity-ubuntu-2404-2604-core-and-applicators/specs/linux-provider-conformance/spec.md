## ADDED Requirements

### Requirement: Ubuntu 24.04 and 26.04 advertise a shared core-and-applicator union
The system SHALL advertise the same non-Pro amd64 capability union on exact Ubuntu 24.04 and Ubuntu 26.04 only when each release has its own complete passing provider-matrix rows. Ubuntu 24.04 SHALL gain any portable package capabilities already proved on Ubuntu 26.04. Ubuntu 26.04 SHALL gain applicator capabilities already proved on Ubuntu 24.04. Exact release identity SHALL remain required; inheritance across releases or derivatives SHALL NOT advertise rows.

#### Scenario: Ubuntu 24.04 advertises portable flatpak and PWA after exact evidence
<!-- verification-id: OS-LPC-033 -->
- **WHEN** Ubuntu 24.04 amd64 has complete passing flatpak and PWA (chromium and google-chrome) rows and an endpoint reports matching Ubuntu 24.04 facts with observed flatpak/browser backends
- **THEN** the production capability document advertises `provider:package/flatpak` and `provider:package/pwa` for that endpoint and does not manufacture those capabilities for another Ubuntu release or architecture

#### Scenario: Ubuntu 26.04 advertises an applicator after exact evidence
<!-- verification-id: OS-LPC-034 -->
- **WHEN** an applicator capability already passing on Ubuntu 24.04 amd64 also has a complete passing Ubuntu 26.04 amd64 row and an endpoint reports matching Ubuntu 26.04 facts
- **THEN** the production capability document advertises that applicator capability for the Ubuntu 26.04 endpoint without treating the Ubuntu 24.04 row as sufficient evidence

#### Scenario: Incomplete union side remains fail-closed
<!-- verification-id: OS-LPC-035 -->
- **WHEN** only one Ubuntu LTS has a complete passing row for an applicator or portable package capability
- **THEN** authenticated capability generation advertises it only for the proved release and capability-compatible delivery stays fail-closed for the unproved release
