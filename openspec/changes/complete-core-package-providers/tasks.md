## 1. Establish Package Traceability and Fixtures

- [ ] 1.1 Register OS-PRM-019 through OS-PRM-028 and reconcile OS-PRM-001 through OS-PRM-018 in `test/traceability.yaml` with configuration, provider-contract, exact-argv, container, redaction, lock, and activation selectors.
- [x] 1.2 Freeze current APT, Pacman, `yay` rejection, APT repository/key, provider-matrix, and capability-advertisement fixtures so every vertical slice begins from an independently known baseline.
- [x] 1.3 Define and document the qualifying Debian 12, Ubuntu 24.04, and Arch 2026-07-06 amd64 rows, backend-specific supported intent, required evidence layers, and provider-contract revision.
- [x] 1.4 Add deterministic test-only package sources with two known versions and signed APT/Pacman metadata, mismatched/unknown keys, unrelated native configuration, and a controlled AUR-compatible build fixture.
- [x] 1.5 Verify fixture package payloads, versions, signatures, and checksums independently of the provider implementation and fail the harness if any fixture becomes mutable.

## 2. Complete the Shared Package Contract

- [x] 2.1 For OS-PRM-001, OS-PRM-002, OS-PRM-005, and OS-PRM-006, name the configuration/provider seams and selected evidence, write and observe focused red lifecycle/policy table cases, then implement only the missing provider-aware validation.
- [x] 2.2 Add bounded fuzz properties and committed seed regressions for package lifecycle, version, provider, architecture, policy, and provider-option parsing/normalization.
- [x] 2.3 Write and observe a red two-resource concurrency test for OS-PRM-010 with an injected coordinator, then place APT and Pacman/AUR mutations into their engine-scoped native package lock domains without wall-clock sleeps.
- [x] 2.4 Add context-cancellation, bounded lock-timeout, native-lock-contention, and sanitized failure cases at the provider seam.
- [ ] 2.5 Write exact process-boundary tests for noninteractive argv, sanitized environment, bounded stdout/stderr, and absence of shell execution for every native command introduced or changed.
- [x] 2.6 Reconcile package activation signals with the common execution result so notifications and reboot-required state remain observable without implicit service activation or reboot.

## 3. Qualify APT on Debian and Ubuntu

- [x] 3.1 For OS-PRM-001, write and observe one focused red APT absence/purge provider-contract case, then complete present, absent, and purged convergence with a compliant second Check.
- [ ] 3.2 For OS-PRM-003 through OS-PRM-005, write and observe focused red exact-version, unavailable-version, upgrade-blocked, downgrade-blocked, and permitted-downgrade cases before completing APT's provider-native version behavior.
- [ ] 3.3 For OS-PRM-006, write and observe focused red hold/unhold cases and complete idempotent native hold convergence without changing an otherwise compliant package.
- [ ] 3.4 Prove APT cache refresh is coalesced once after changed repository dependencies and before the first dependent resolution; add refresh-failure consistency and no-unrequested-refresh cases.
- [ ] 3.5 Add exact-argv and environment assertions for APT query, compare, install, remove, purge, autoremove, hold, unhold, and refresh boundaries, including the complete noninteractive policy.
- [ ] 3.6 Run the provider contract and actual native package manager against the controlled signed source in Debian 12 amd64 for compliant, drifted, Apply, second Check, removal, exact-version, downgrade, lock, failure, and reboot-marker cases.
- [ ] 3.7 Run the same complete APT contract independently in Ubuntu 24.04 amd64 and retain release-specific selectors rather than inheriting Debian's result.

## 4. Qualify Pacman on Pinned Arch

- [ ] 4.1 Rename/split the current misleading Arch implementation so the native repository provider is registered, described, and reported only as Pacman; preserve existing Pacman-authored configuration compatibility.
- [ ] 4.2 For OS-PRM-001 and OS-PRM-002, write and observe focused red Pacman presence/removal and unsupported-purge cases, then complete lifecycle convergence and second-Check behavior.
- [ ] 4.3 For OS-PRM-027 and OS-PRM-028, write and observe the red case where an unversioned `pacman -S` could choose the wrong version, then install only the repository-resolved exact artifact.
- [ ] 4.4 Add provider-native `vercmp` upgrade/downgrade policy, unavailable-version, resolution-change, and package-database consistency cases for OS-PRM-003 through OS-PRM-005.
- [ ] 4.5 Add exact-argv, sanitized-environment, bounded-diagnostic, shared-lock, cancellation, metadata-refresh, dependency-removal, and activation-result tests at the Pacman process boundary.
- [ ] 4.6 Run the complete provider contract against the actual Pacman database and controlled signed repository in the pinned Arch 2026-07-06 amd64 image, including preservation and compliant second Check.

## 5. Implement the Truthful AUR Provider

- [ ] 5.1 For OS-PRM-019, write and observe red missing-`yay`, missing-build-user, and wrong-platform cases, then implement a distinct AUR provider that returns `unsupported` without provider fallback.
- [ ] 5.2 Add schema/configuration red cases for OS-PRM-026, then accept only typed AUR package intent and continue rejecting shell commands, PKGBUILD bodies, arbitrary build flags, and APT/Pacman-only policy.
- [ ] 5.3 For OS-PRM-024, write and observe a red effective-user/process-boundary test, then resolve and build in a bounded workspace as the declared unprivileged build identity.
- [ ] 5.4 Record sanitized source/package identity and the produced artifact digest, then install the exact artifact through the privileged Pacman boundary under the shared package lock.
- [ ] 5.5 For OS-PRM-025, add red exact-version unavailable, build failure, artifact mismatch, install failure, cancellation, and cleanup cases; prove no other version is installed and transient material follows cleanup policy.
- [ ] 5.6 Run compliant, drifted, Apply, second Check, absence, unsupported, exact-version, failure, process-user, lock, and cleanup evidence with the controlled AUR-compatible fixture in the pinned Arch image.
- [ ] 5.7 Remove the authoring-time `yay` rejection and advertise AUR capabilities only after the exact pinned-Arch evidence row is passing.

## 6. Complete APT Repository and Signing-Key Evidence

- [ ] 6.1 For OS-PRM-013, write and observe red complete-fingerprint mismatch and malformed-key cases, then verify/dearmor/atomically activate only matching key material in the scoped Remotr-owned APT keyring.
- [ ] 6.2 For OS-PRM-011 and OS-PRM-012, write and observe red compliant, drifted, disabled, and absent cases, then complete canonical atomic ownership of APT source, preference, and provider-supported credential fragments.
- [ ] 6.3 Prove unrelated APT sources, preferences, keyrings, and credentials survive Apply and removal; add secret canaries covering desired state, argv, logs, reports, diagnostics, rollback data, and generated files.
- [ ] 6.4 For OS-PRM-014 and OS-PRM-015, prove key-before-repository-before-refresh-before-package dependency order, one coalesced refresh, and sanitized authenticated-repository failure.
- [ ] 6.5 Run the complete APT repository/key contract independently in Debian 12 and Ubuntu 24.04 containers against the controlled signed repository and retain exact selectors for both releases.

## 7. Add Pacman Repository and Signing Trust

- [ ] 7.1 Define typed Pacman signing-key and repository models, lifecycle, dependency, ownership, signature-level, architecture, server, and credential-reference fields with configuration/parser round-trip tests and fuzz boundaries.
- [ ] 7.2 For OS-PRM-021 and OS-PRM-022, write and observe red matching, mismatched, unknown, absent, and unrelated-key preservation cases, then implement exact-fingerprint verification and narrowly declared provider-native trust mutation.
- [ ] 7.3 For OS-PRM-020, write and observe red compliant, drifted, malformed, disabled, and absent repository cases, then stage and provider-validate canonical Remotr fragments before atomic activation.
- [ ] 7.4 Implement one idempotently owned `pacman.conf` include boundary, preserve unrelated content byte-for-byte where unmanaged, and remove the boundary only when no managed repository requires it.
- [ ] 7.5 For OS-PRM-018, prove signing trust, repository activation, one metadata refresh, exact package convergence, preservation, and compliant second Check through the declared dependency graph.
- [ ] 7.6 Add secret-canary, exact-argv, sanitized-environment, lock, cancellation, native-validation failure, partial-write recovery, and removal evidence for the Arch trust/repository boundary.
- [ ] 7.7 Run the complete repository/signing-trust contract against the actual Pacman configuration, keyring, database, and controlled signed repository in the pinned Arch image.

## 8. Make Evidence Control Advertisement

- [ ] 8.1 Replace aggregate `untested` package/repository selectors with exact executable selectors for APT package/repository on Debian 12 and Ubuntu 24.04 and Pacman, AUR, and Pacman repository/trust on pinned Arch.
- [ ] 8.2 For OS-PRM-023, write and observe a red forged-passing-row case, then require every passing row to resolve and run the complete evidence set for its exact distribution, release, architecture, backend, contract revision, and environment.
- [ ] 8.3 Publish granular lifecycle, version, policy, repository, trust, and AUR capability fields only from matching passing rows and prove deferred DNF/RPM/APK/Zypper/Snap rows remain unadvertised.
- [ ] 8.4 Add configuration release-validation cases proving a missing, stale, partial, or mismatched row is rejected before artifact release and an agent with mismatched local discovery fails closed.
- [ ] 8.5 Add one public composed workflow per qualifying distribution that installs verified trust/repository state and converges an exact package without committing generated `desired.yaml` or `crons.yaml`.

## 9. Verify and Close the Package Gate

- [ ] 9.1 Run each focused test before and after its minimum red-green slice, then run provider contracts, exact-argv suites, parser/config fuzzing, secret-canary tests, and deterministic lock/concurrency tests.
- [ ] 9.2 Run Debian 12, Ubuntu 24.04, and pinned Arch real-provider container targets from clean state and retain evidence for compliant, drifted, Apply, second Check, absence, version, trust, repository, failure, preservation, and cleanup behavior.
- [ ] 9.3 Run focused mutation tests for provider selection, exact-version enforcement, downgrade policy, fingerprint verification, trust-before-repository ordering, AUR privilege separation, and advertisement gating with no unexplained relevant survivor.
- [ ] 9.4 Run `make test`, strict OpenSpec validation, traceability validation, documentation validation, and configuration composition/validation after the complete matrix passes.
- [ ] 9.5 Mark only the exact completed provider rows passing, close umbrella package tasks 6.4 and 6.10, and leave every excluded provider explicitly deferred and unadvertised.
