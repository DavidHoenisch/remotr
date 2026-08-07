## Context

Remotr advertises capabilities only from exact passing provider-matrix rows. Ubuntu 24.04 and Ubuntu 26.04 currently diverge:

- Ubuntu 24.04 has the broad M1–M5 applicator surface plus core package/file/download/bootstrap/command/systemd.
- Ubuntu 26.04 has core delivery plus flatpak/PWA, but lacks most typed applicators.

Operators want release-interchangeable delivery for those non-Pro capabilities. Exact-release evidence rules forbid treating a 24.04 row as proof for 26.04 (or the reverse).

Pop!_OS stays out of this change by decision; it follows once the Ubuntu union is frozen.

## Goals / Non-Goals

**Goals:**

- Produce a shared Ubuntu amd64 capability union for 24.04 and 26.04 covering every currently passing applicator/core row from either release (except Ubuntu Pro).
- Ship in sequenced waves with red→green capability-document tests and real selectors.
- Reuse existing provider-contract bodies behind release-specific selectors and test names.

**Non-Goals:**

- Qualifying Pop!_OS, Debian, Arch, or ARM in this change.
- Soft inheritance / family-based advertisement across Ubuntu releases.
- Changing applicator semantics, config schema, or Sync authentication.
- Claiming Ubuntu Pro on any derivative or non-exact Ubuntu identity.

## Decisions

- One OpenSpec change with phased tasks rather than separate changes, so the destination union stays explicit while waves still land independently.
- Phase A first: add Ubuntu 24.04 flatpak + PWA backends using the established Ubuntu 26.04 VM contract pattern and a 24.04-pinned core-delivery/portable selector.
- Phase B next: copy each Ubuntu 24.04 passing applicator identity onto `ubuntu`/`26.04`/`amd64` only after a 26.04 selector proves guest identity and contract behavior.
- Wave packaging for Phase B:
  1. Container wave: directory, link, knownHost, endpointSchedule/cron (+ package/repository/file/download if any still missing).
  2. Host/system wave: sysctl, kernelModule, hostname, hostLocale, timeSync, mount, swap, journald, logrotate, certificate, trustAnchor, appArmorProfile, auditRules, reboot, systemdUnit, service, endpointSchedule/systemd-timer.
  3. User/auth wave: user, group, authorizedKey, sudo, userFile, accountLimit, loginPolicy.
  4. Network wave: hostsEntry, dnsResolver, route, networkProfile, firewall backends.
  5. Desktop wave: desktopSetting, sessionPolicy, browserPolicy backends.
- Negative capability checks for ARM and the other Ubuntu release remain mandatory for every wave.
- PopOS follow-on is explicitly deferred.

## Risks / Trade-offs

- [Ubuntu 26.04 VM image drift / selector cost] → Pin box versions; assert `/etc/os-release` and amd64 inside guests; reuse harness patterns from core delivery.
- [Partial waves leave fleets asymmetric mid-rollout] → Document wave boundaries in operator notes; capability_blocked remains truthful.
- [Desktop/browser facts must be observed before advertisement] → Keep `rowAppliesToEndpoint` / observed-fact rules unchanged.
- [Large matrix growth increases agent binary size] → Acceptable; matrix is already embedded.

## Migration Plan

1. Land Phase A; stamp agent; Ubuntu 24.04 endpoints gain flatpak/PWA when observed.
2. Land Phase B waves; stamp agent after each wave or after the full B sequence based on release discipline.
3. Open the deferred PopOS follow-on change named `parity-popos-2404-ubuntu-union` to mirror this frozen Ubuntu core-and-applicator union onto exact Pop!_OS 24.04 rows (no soft inheritance from Ubuntu).
4. Update support docs/traceability for the shared Ubuntu surface.

## Open Questions

None for planning. Wave B execution may reveal container-vs-VM environment mismatches for individual resources; those stay fail-closed until proved. Current known deferral: Ubuntu 26.04 `swap` (`swapon` Invalid argument on cloud-image guests in this harness). Deferred next change (after archive): `parity-popos-2404-ubuntu-union`.
