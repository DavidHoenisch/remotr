## ADDED Requirements

### Requirement: Arch 2026-07-06 core delivery contracts may be advertised from exact evidence
The system SHALL advertise the Arch 2026-07-06 amd64 unblock capability set only when complete exact `arch` / `2026-07-06` / `amd64` provider-matrix rows pass for those contracts. The unblock set is `provider:package/pacman`, `provider:init/systemd`, `provider:package/flatpak`, `provider:package/pwa`, `provider:schedule/systemd-timer`, `resource:package`, `resource:file`, `resource:download`, `resource:bootstrap`, `resource:command`, `resource:systemd`, `resource:service`, `resource:endpoint-schedule`, and `resource:user-file`. Exact Arch identity SHALL remain distinct from Ubuntu, Debian, and Pop!_OS; Ubuntu Pro and APT capabilities SHALL remain absent.

#### Scenario: Qualified Arch 2026-07-06 advertises the unblock set
<!-- verification-id: OS-LPC-036 -->
- **WHEN** an endpoint reports exact Arch 2026-07-06 amd64 facts (including Omarchy mapped to Arch) with observed flatpak/browser backends and the corresponding unblock provider-matrix rows are complete and passing
- **THEN** the production capability document advertises those providers and resources and does not advertise Ubuntu Pro, APT, or unrelated unproven Arch architecture rows

#### Scenario: Unqualified Arch architecture stays fail-closed
<!-- verification-id: OS-LPC-037 -->
- **WHEN** an endpoint reports exact Arch on another architecture without matching passing rows
- **THEN** the production capability document omits the unblock provider and resource capabilities for that endpoint
