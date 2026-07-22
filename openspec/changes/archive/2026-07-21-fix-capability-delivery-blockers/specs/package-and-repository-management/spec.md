## MODIFIED Requirements

### Requirement: Initial and extended provider gates are explicit
Before this umbrella closes, exact native-package convergence SHALL be qualified for APT on Debian 12 amd64, Ubuntu 24.04 amd64, and Ubuntu 26.04 amd64 and for Pacman plus a truthful AUR/`yay` provider on the pinned Arch 2026-07-06 amd64 release. APT repository/key behavior on Debian and each advertised Ubuntu release and Pacman repository/signing-trust behavior on Arch SHALL be qualified in the same closeout. Every advertised row SHALL pass provider-contract compliant, drifted, Apply, second-Check, absence, exact-version, failure, lock, redaction, and preservation evidence applicable to that provider in the named real environment. A row SHALL remain unadvertised until its complete provider-contract revision and exact distribution evidence passes; shared configuration validation SHALL reject a configuration that targets an unadvertised row before release creation. Fedora/RHEL, DNF4/DNF5, RPM repositories, image-based RPM systems, APK, Zypper, Snap, and other immutable-image providers SHALL remain unadvertised until future OpenSpec changes define and test their complete matrices.

#### Scenario: Unreleased provider is authored
<!-- verification-id: OS-PRM-031 -->
- **WHEN** configuration selects a provider not present in the target agent capability matrix
- **THEN** release validation fails rather than accepting a future placeholder

#### Scenario: Arch repository supplies a managed package
<!-- verification-id: OS-PRM-032 -->
- **WHEN** a package on the pinned Arch release depends on a Remotr-owned repository and declared signing trust
- **THEN** the provider verifies and activates that trust and repository, refreshes metadata once, converges the package, preserves unrelated Pacman configuration, and reports a compliant second Check

#### Scenario: A passing row lacks executable evidence
<!-- verification-id: OS-PRM-033 -->
- **WHEN** a provider matrix row is marked `passing` without selectors proving its complete required contract in the exact distribution, release, architecture, backend, contract revision, and environment
- **THEN** matrix validation fails and the capability is not advertised

#### Scenario: Ubuntu 26.04 APT row passes
<!-- verification-id: OS-PRM-029 -->
- **WHEN** APT package and applicable repository/key contracts pass their selected real Ubuntu 26.04 amd64 evidence
- **THEN** only those exact provider revisions are included in the frozen release catalog for Ubuntu 26.04 amd64

#### Scenario: Ubuntu 26.04 APT row is missing
<!-- verification-id: OS-PRM-030 -->
- **WHEN** configuration targets APT package or repository behavior on Ubuntu 26.04 amd64 before the applicable row has complete evidence
- **THEN** `config validate` and server Git-sync validation reject the source with the same target/provider diagnostic before Release ref advance
