## 1. Capability Qualification

- [x] 1.1 For OS-LPC-031 and OS-LPC-032, add the production capability-document test that requires the PopOS 24.04 amd64 unblock set and keeps ARM/other releases fail-closed; record the intended red failure.
- [x] 1.2 Add exact `popos` / `24.04` / `amd64` provider-matrix rows for package/apt, file, download, bootstrap, command, systemd, flatpak, and pwa, then make the focused capability-document tests green.

## 2. Provider Evidence

- [x] 2.1 Add Ubuntu-24.04-based container fixture with remapped `ID=pop` / `VERSION_ID=24.04`, plus apt/file/download selectors that assert PopOS identity.
- [x] 2.2 Add PopOS-specific VM provider-contract test entry points and a Vagrant core-delivery selector on Ubuntu 24.04 cloud image with remapped PopOS identity; run and retain cleanup evidence where the environment allows.

## 3. Traceability and Verification

- [x] 3.1 Record OS-LPC-031/030 in OpenSpec, traceability, and focused documentation without advertising Ubuntu Pro on PopOS.
- [x] 3.2 Run focused capability/provider tests and `make test`; validate the OpenSpec change.
