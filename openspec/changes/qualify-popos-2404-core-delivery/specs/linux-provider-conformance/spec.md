## ADDED Requirements

### Requirement: PopOS 24.04 core delivery contracts may be advertised from exact evidence
The system SHALL advertise the Pop!_OS 24.04 LTS amd64 unblock capability set only when complete exact `popos` / `24.04` / `amd64` provider-matrix rows pass for those contracts. The unblock set is `provider:package/apt`, `provider:init/systemd`, `provider:package/flatpak`, `provider:package/pwa`, `resource:package`, `resource:file`, `resource:download`, `resource:bootstrap`, `resource:command`, and `resource:systemd`. Exact Pop!_OS identity SHALL remain distinct from Ubuntu and Debian; Ubuntu Pro and other Ubuntu-only capabilities SHALL remain absent.

#### Scenario: Qualified PopOS 24.04 advertises the unblock set
<!-- verification-id: OS-LPC-031 -->
- **WHEN** an endpoint reports exact Pop!_OS 24.04 LTS amd64 facts and the corresponding unblock provider-matrix rows are complete and passing
- **THEN** the production capability document advertises those providers and resources and does not advertise Ubuntu Pro or unrelated unproven PopOS release/architecture rows

#### Scenario: Unqualified PopOS release stays fail-closed
<!-- verification-id: OS-LPC-032 -->
- **WHEN** an endpoint reports exact Pop!_OS on another release or architecture without matching passing rows
- **THEN** the production capability document omits the unblock provider and resource capabilities for that endpoint
