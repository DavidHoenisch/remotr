## 1. Capability Qualification

- [x] 1.1 Add the OS-LPC-029 production capability-document test, record the intended missing-capability red result, and constrain the claim against ARM and another Ubuntu release.
- [x] 1.2 Add exact Ubuntu 24.04 LTS amd64 provider-matrix rows for `bootstrap-v1`, `command-v1`, and `systemd-v1` and make the focused test green.

## 2. Provider Evidence

- [x] 2.1 Add release-specific command, bootstrap, and systemd VM provider-contract test entry points without weakening the existing Ubuntu 26.04 evidence.
- [x] 2.2 Add and run the pinned Ubuntu 24.04 LTS amd64 Vagrant selector, including exact guest identity assertions and verified cleanup.

## 3. Traceability and Release Validation

- [x] 3.1 Record OS-LPC-029 in OpenSpec, traceability, TDD evidence, and Ubuntu 24.04 support documentation without broadening the typed M1-M5 inventory.
- [x] 3.2 Run strict OpenSpec validation, traceability lint, the Ubuntu qualification audit, focused tests, and `make test`; monitor the pull request quality gates through merge.
- [ ] 3.3 Merge only after every required pull request check passes, then stamp and verify the next patch release.
