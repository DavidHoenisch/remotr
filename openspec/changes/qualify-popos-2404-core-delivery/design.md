## Context

Remotr derives endpoint capability documents from exact passing provider-matrix rows. After `separate-applicability-and-compliance`, Pop!_OS reports exact `PopOS`/`popos` identity with Debian family lineage and does not inherit Ubuntu or Debian qualification rows. Production therefore fails closed for Pop!_OS 24.04 until exact `popos` rows exist.

Observed enrolled endpoint `pop-os-4532e745` is blocked on the unblock capability set required by the current engineering Release. Healthy siblings are Ubuntu 26.04 endpoints that already advertise those contracts.

No published Pop!_OS cloud/Vagrant box is available in the controlled environment. Operators accepted Ubuntu 24.04 package/container and cloud-image bases with remapped `/etc/os-release` identity (`ID=pop`, `VERSION_ID=24.04`) as the evidence source for this qualification wave.

## Goals / Non-Goals

**Goals:**

- Advertise the exact unblock set for Pop!_OS 24.04 LTS amd64 once matrix rows and selectors pass.
- Preserve fail-closed behavior for absent PopOS rows, other PopOS releases/architectures, and Ubuntu Pro.
- Keep Debian and Ubuntu qualification independent; PopOS rows never satisfy Debian/Ubuntu identity predicates.
- Reuse existing provider-contract bodies behind PopOS-specific selectors and test names.

**Non-Goals:**

- Full M1–M5 applicator parity with Ubuntu 24.04 beyond the unblock set.
- Authentic System76-distributed ISO/cloud image pinning in this change.
- Qualifying ARM, other Pop!_OS releases, or Ubuntu Pro on Pop!_OS.
- Changing artifact composition, enrollment, Sync authentication, or Config `targetDistros` grammar.

## Decisions

- Add exact `distribution: popos` matrix rows instead of a Debian-family inheritance rule. Alternatives considered: mapping PopOS to Ubuntu 24.04 rows (rejected; breaks Ubuntu-only gates and prior exact-identity work) and leaving fail-closed (rejected; does not restore fleet delivery).
- Use Ubuntu 24.04 container/cloud bases with remapped `/etc/os-release` and assert `ID=pop`, `VERSION_ID=24.04`, amd64 inside fixtures. Alternatives considered: custom Pop!_OS box from ISO (deferred; longer path) and live enrolled endpoint evidence (rejected; not disposable/controlled).
- Split selectors: container for `package`/`file`/`download`; VM core-delivery for `command`/`bootstrap`/`systemd`/`flatpak`/`pwa`, matching Ubuntu 26.04 evidence partitioning.
- Register OS-LPC-029 as the positive PopOS qualification scenario; leave OS-LPC-028 as the fail-closed absence scenario.

## Risks / Trade-offs

- [Remapped Ubuntu base is not a System76 image] → Document the fixture strategy; keep identity assertions strict; defer authentic box pinning to a follow-up.
- [Partial row sets leave endpoints blocked on one missing requirement] → Publish the full unblock set in one change; negative capability tests assert ARM and other releases stay dark.
- [Installed agents keep the old embedded matrix] → Stamp and deploy a new agent after merge; desire upgrade on the Pop endpoint.
- [Config still targets exact Debian while PopOS is exact] → Delivery for Debian-only branches remains inapplicable; operators must include `PopOS` or use portable (unscoped) resources.

## Migration Plan

1. Land matrix rows, fixtures, capability-document tests, and selectors.
2. Merge after gates pass; stamp the next agent patch release embedding the matrix.
3. Desire agent upgrade on Pop!_OS endpoints; confirm `distro=PopOS` labels and cleared `capability_blocked`.
4. Ensure configuration repositories that intend Pop!_OS coverage list `PopOS` in `targetDistros` where exact targeting is used.

## Open Questions

None for this change. Authentic Pop!_OS cloud-image pinning remains a future tightening.
