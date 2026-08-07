## 1. Phase A — Ubuntu 24.04 portable packages

- [x] 1.1 For OS-LPC-033, extend the production capability-document test so Ubuntu 24.04 amd64 requires flatpak and PWA (chromium and google-chrome) and remains fail-closed for ARM/other releases; record the intended red failure.
- [x] 1.2 Add exact Ubuntu 24.04 amd64 matrix rows for flatpak and both PWA backends with VM selectors; make the focused capability test green.
- [x] 1.3 Add/extend Ubuntu 24.04 VM harness coverage for flatpak/PWA provider contracts with exact Ubuntu 24.04 identity assertions; run when the environment allows.

## 2. Phase B1 — Ubuntu 26.04 container applicators

- [x] 2.1 For OS-LPC-034/035, add a focused Ubuntu 26.04 capability-document wave test for directory, link, knownHost, and endpointSchedule/cron; record red.
- [x] 2.2 Add exact Ubuntu 26.04 amd64 matrix rows and container selectors for that wave; make the focused test green and run container evidence.

## 3. Phase B2 — Ubuntu 26.04 host/system applicators

- [x] 3.1 Add focused capability-document coverage for the host/system wave (sysctl, kernelModule, hostname, hostLocale, timeSync, mount, swap, journald, logrotate, certificate, trustAnchor, appArmorProfile, auditRules, reboot, systemdUnit, service, endpointSchedule/systemd-timer); record red then green with exact Ubuntu 26.04 rows/selectors.
- [x] 3.2 Wire and run the corresponding Ubuntu 26.04 VM selectors with identity assertions and cleanup. Swap remains deferred (`swapon` Invalid argument on 26.04 cloud image); firewalld/nft restore fixed for system-safety.

## 4. Phase B3 — Ubuntu 26.04 user/auth applicators

- [x] 4.1 Add focused capability-document coverage for user, group, authorizedKey, sudo, userFile, accountLimit, and loginPolicy; land exact Ubuntu 26.04 rows/selectors red→green.
- [x] 4.2 Wire and run the Ubuntu 26.04 user/auth VM evidence selectors.

## 5. Phase B4 — Ubuntu 26.04 network applicators

- [x] 5.1 Add focused capability-document coverage for hostsEntry, dnsResolver, route, networkProfile, and firewall backends; land exact Ubuntu 26.04 rows/selectors red→green.
- [x] 5.2 Wire and run the Ubuntu 26.04 network-recovery VM evidence selectors.

## 6. Phase B5 — Ubuntu 26.04 desktop applicators

- [x] 6.1 Add focused capability-document coverage for desktopSetting, sessionPolicy, and browserPolicy backends; land exact Ubuntu 26.04 rows/selectors red→green.
- [x] 6.2 Wire and run the Ubuntu 26.04 desktop-session VM evidence selectors.

## 7. Verification and handoff

- [x] 7.1 Assert Ubuntu 24.04 and Ubuntu 26.04 advertise equal non-Pro capability ID sets for equivalent observed facts in a focused parity test.
- [x] 7.2 Update main `linux-provider-conformance` spec, traceability (OS-LPC-033 through OS-LPC-035 plus wave selectors), and support notes; run focused suites and `make test`.
- [x] 7.3 Record the deferred PopOS union follow-on as an explicit next change once this Ubuntu union is frozen.
