## Why

Enrolled Arch/Omarchy amd64 endpoints (for example `bot-ad3b9950`) remain `capability_blocked` on the current fleet Release because exact `arch` / `2026-07-06` provider rows are absent for the common POSIX/systemd and portable contracts. Exact Arch identity correctly refuses to inherit Ubuntu/Debian/Pop!_OS qualification, so those resources never match until Arch-specific evidence exists.

## What Changes

- Qualify the unblock set for exact Arch 2026-07-06 amd64: `provider:package/pacman`, `provider:init/systemd`, `provider:package/flatpak`, `provider:package/pwa`, `provider:schedule/systemd-timer`, and resources `package`, `file`, `download`, `bootstrap`, `command`, `systemd`, `service`, `endpoint-schedule`, and `user-file`.
- Publish only those exact `distribution: arch` / `release: "2026-07-06"` / `architecture: amd64` passing rows in the production capability catalog.
- Keep Ubuntu Pro and APT providers unadvertised on Arch.
- Record deterministic capability-document, provider-selector, and traceability evidence for the new support boundary.
- Stamp a new agent release after merge so enrolled Arch/Omarchy endpoints advertise the updated embedded matrix without re-enrollment.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `linux-provider-conformance`: Add an exact Arch 2026-07-06 amd64 qualification scenario for the core delivery unblock set while preserving fail-closed behavior when Arch rows are absent.

## Impact

- Provider matrix and generated endpoint capability documents gain exact Arch 2026-07-06 amd64 rows for the unblock set.
- Container and Vagrant fixtures gain Arch identity assertions using the pinned Arch OCI image and `generic/arch` libvirt box.
- Traceability and support documentation gain OS-LPC-036/037 evidence for Arch core delivery.
- Config repos that intentionally target `Arch` (or portable resources) can deliver once agents embed the new matrix.
