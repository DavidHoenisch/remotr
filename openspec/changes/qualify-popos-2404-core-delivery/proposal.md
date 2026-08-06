## Why

Enrolled Pop!_OS 24.04 LTS amd64 endpoints (for example `pop-os-4532e745`) remain `capability_blocked` on the current fleet Release because exact `popos` provider rows are absent. Exact Pop!_OS identity correctly refuses to inherit Ubuntu/Debian qualification, so portable and PopOS-targeted delivery resources never match until PopOS-specific evidence exists.

## What Changes

- Qualify the unblock set for exact Pop!_OS 24.04 LTS amd64: `provider:package/apt`, `provider:init/systemd`, `provider:package/flatpak`, `provider:package/pwa`, and resources `package`, `file`, `download`, `bootstrap`, `command`, and `systemd`.
- Publish only those exact `distribution: popos` / `release: "24.04"` / `architecture: amd64` passing rows in the production capability catalog.
- Keep Ubuntu Pro and other Ubuntu-only providers unadvertised on Pop!_OS.
- Record deterministic capability-document, provider-selector, and traceability evidence for the new support boundary.
- Stamp a new agent release after merge so enrolled Pop!_OS endpoints advertise the updated embedded matrix without re-enrollment.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `linux-provider-conformance`: Add an exact Pop!_OS 24.04 LTS amd64 qualification scenario for the core delivery unblock set while preserving fail-closed behavior when PopOS rows are absent.

## Impact

- Provider matrix and generated endpoint capability documents gain exact PopOS 24.04 amd64 rows for the unblock set.
- Container and Vagrant fixtures gain PopOS identity assertions using Ubuntu 24.04 bases with remapped `/etc/os-release` (`ID=pop`, `VERSION_ID=24.04`).
- Traceability and support documentation gain OS-LPC-029 evidence for PopOS core delivery.
- Config repos that intentionally target `PopOS` (or portable resources) can deliver once agents embed the new matrix.
