## Why

Ubuntu 24.04 LTS amd64 and Ubuntu 26.04 LTS amd64 advertise different provider-matrix surfaces today: 24.04 carries the broad applicator set, while 26.04 carries core delivery plus portable flatpak/PWA. Mixed fleets cannot treat those releases interchangeably, and configuration that is portable or release-spanning remains capability-blocked on one side or the other.

## What Changes

- Backfill Ubuntu 24.04 with the Ubuntu 26.04-only portable package capabilities: `flatpak` and `pwa` (chromium and google-chrome backends).
- Qualify Ubuntu 26.04 with the Ubuntu 24.04 passing applicator set (exact `ubuntu` / `26.04` / `amd64` rows) in evidence-backed waves.
- Keep Ubuntu Pro and other Ubuntu-only identity gates unchanged.
- Leave Pop!_OS out of this change; a follow-on change mirrors the resulting union onto PopOS 24.04.
- Record capability-document, selector, and traceability evidence for each wave before advertising rows.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `linux-provider-conformance`: Require release-exact Ubuntu 24.04/26.04 capability parity for the shared core-and-applicator union without inheritance across releases or derivatives.

## Impact

- Provider matrix and embedded agent capability catalogs gain new exact Ubuntu 24.04 and Ubuntu 26.04 rows.
- Makefile/Vagrant/container selectors grow Ubuntu 26.04 applicator evidence waves and Ubuntu 24.04 portable-app VM selectors.
- Traceability gains OS-LPC verification IDs for each parity wave.
- After merge and agent stamp/deploy, Ubuntu 24.04 and 26.04 endpoints can receive the same non-Pro resource surface when facts and observed backends match.
