## ADDED Requirements

### Requirement: Ubuntu 24.04 core delivery qualification is exact
Remotr SHALL advertise the command, bootstrap, and systemd core delivery contracts for Ubuntu 24.04 LTS amd64 only after their public provider contracts pass on a pinned exact-release VM and the production capability document derives the matching rows from exact endpoint facts.

#### Scenario: Ubuntu 24.04 core delivery contracts are qualified
<!-- verification-id: OS-LPC-029 -->
- **WHEN** command, bootstrap, and systemd pass their provider contracts on the pinned Ubuntu 24.04 LTS amd64 VM and the production capability document is generated from exact Ubuntu 24.04 amd64, systemd, and APT facts
- **THEN** the frozen catalog advertises `command-v1`, `bootstrap-v1`, and `systemd-v1` only for that exact target without manufacturing support for another release or architecture
