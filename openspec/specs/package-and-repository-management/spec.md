# Package and Repository Management Specification

## Purpose

Define safe, convergent Linux package, repository, and signing-trust management with truthful provider capabilities and verifiable release gates.

## Requirements

### Requirement: Native package lifecycle is explicit
The package resource SHALL support explicit `present`, `absent`, and provider-supported `purged` lifecycle. Package removal SHALL not be inferred from omission.

#### Scenario: Package is absent
<!-- verification-id: OS-PRM-001 -->
- **WHEN** a package declares `absent` and the package is installed
- **THEN** Check reports drift and Apply removes the package using the selected provider

#### Scenario: Purge is unsupported
<!-- verification-id: OS-PRM-002 -->
- **WHEN** a package declares `purged` on a provider without purge semantics
- **THEN** validation rejects that lifecycle/provider combination

### Requirement: Managed versions converge
When a native package version or version constraint is specified, the provider SHALL compare the installed version and SHALL install, upgrade, or downgrade as required by the declared policy. When version is omitted, any installed version satisfies `present`.

#### Scenario: Exact version differs
<!-- verification-id: OS-PRM-003 -->
- **WHEN** a package requires an exact version and another version is installed
- **THEN** Check reports both safe versions and Apply converges to the requested version if available

#### Scenario: Requested version is unavailable
<!-- verification-id: OS-PRM-004 -->
- **WHEN** no configured repository offers a requested version
- **THEN** Apply fails with a non-secret provider reason and leaves the package database consistent

### Requirement: Transaction policy is declared
The package contract SHALL explicitly represent allowed upgrade, downgrade, hold/pin, cache-refresh, and dependency-removal behavior, and SHALL reject policy unsupported by the selected provider.

#### Scenario: Downgrade not permitted
<!-- verification-id: OS-PRM-005 -->
- **WHEN** the installed version is newer and the resource does not permit downgrade
- **THEN** the engine reports a policy-blocked result without downgrading

#### Scenario: Package is held
<!-- verification-id: OS-PRM-006 -->
- **WHEN** hold is managed and the native hold state differs
- **THEN** Check reports drift and Apply changes the native hold state

### Requirement: Provider identity is truthful
APT, Pacman, AUR helpers, DNF-family tools, Flatpak, PWA, and Remotr catalog packages SHALL be distinct providers with provider-specific capability matrices. A selected provider SHALL execute only its advertised provider boundary and SHALL return `unsupported` when that boundary's required executable, runtime identity, or platform prerequisites are absent. `yay` SHALL never execute through the Pacman repository provider while being reported as AUR-aware.

#### Scenario: Yay is selected
<!-- verification-id: OS-PRM-007 -->
- **WHEN** a package explicitly selects `yay` on the qualifying pinned Arch row
- **THEN** the agent executes the advertised AUR provider through its unprivileged build boundary or returns `unsupported` without routing the package through the Pacman repository provider

#### Scenario: DNF is advertised
<!-- verification-id: OS-PRM-008 -->
- **WHEN** a Fedora/RHEL endpoint advertises DNF package management
- **THEN** facts, Check, install, removal, version behavior, and integration tests are all available

#### Scenario: DNF is selected during this change
<!-- verification-id: OS-PRM-009 -->
- **WHEN** authored configuration selects `dnf` before a future RPM-family OpenSpec change is complete
- **THEN** validation rejects it with a roadmap diagnostic rather than constructing the existing non-applying stub

#### Scenario: A provider executable is absent
<!-- verification-id: OS-PRM-019 -->
- **WHEN** a package selects a provider whose required executable or runtime identity is absent
- **THEN** Check returns `unsupported` with a sanitized provider reason and no alternative provider is executed

### Requirement: Package operations are locked and noninteractive
Package providers SHALL use argv-based noninteractive commands, provider-native lock handling, bounded timeouts, and sanitized environment. Conflicting package operations SHALL share one lock domain.

#### Scenario: Two package resources drift
<!-- verification-id: OS-PRM-010 -->
- **WHEN** two package changes are ready in the same run
- **THEN** their transactions execute serially under the package lock

### Requirement: Repository lifecycle is first-class
Repository resources SHALL manage a named repository as `present`, `absent`, or disabled, using provider-owned configuration and explicit URL, suites/releases, components, priority, architecture, signature policy, and credential-reference fields supported by that provider. Provider-owned configuration SHALL have a deterministic path and narrowly owned activation boundary, SHALL be validated before atomic activation, and SHALL preserve unrelated native package-manager configuration.

#### Scenario: APT repository is present
<!-- verification-id: OS-PRM-011 -->
- **WHEN** a Debian/Ubuntu repository declaration differs from its Remotr-owned source fragment
- **THEN** Check reports drift and Apply validates and atomically writes the canonical fragment

#### Scenario: Repository is removed
<!-- verification-id: OS-PRM-012 -->
- **WHEN** a named repository declares `absent`
- **THEN** Apply removes only its Remotr-owned repository configuration and preserves unrelated sources and package-manager configuration

#### Scenario: Pacman repository is present
<!-- verification-id: OS-PRM-020 -->
- **WHEN** an Arch repository declaration differs from its Remotr-owned fragment or required include boundary
- **THEN** Check reports safe drift and Apply validates and atomically activates only the canonical owned configuration while preserving unrelated `pacman.conf` content

### Requirement: Repository signing trust is separate and verifiable
Signing-key resources SHALL be separate from repository definitions, SHALL verify the complete requested fingerprint before activation, and SHALL install keys in provider-appropriate scoped keyrings or provider-native trust databases. Trust mutation SHALL be narrowly owned and SHALL not grant undeclared global trust. Repository activation SHALL remain blocked when its signing-key dependency is absent, mismatched, or untrusted.

#### Scenario: Downloaded key fingerprint mismatches
<!-- verification-id: OS-PRM-013 -->
- **WHEN** fetched signing-key material does not match the declared fingerprint
- **THEN** Apply fails before enabling the repository and does not persist the untrusted key

#### Scenario: Pacman signing trust is activated
<!-- verification-id: OS-PRM-021 -->
- **WHEN** an Arch signing-key resource declares a matching fingerprint and a dependent repository requires that key
- **THEN** Apply imports and applies only the declared provider-native trust before activating the repository and reports a compliant second Check

#### Scenario: Pacman signing fingerprint mismatches
<!-- verification-id: OS-PRM-022 -->
- **WHEN** Arch signing-key material does not match the complete declared fingerprint
- **THEN** Apply leaves the Pacman keyring and dependent repository inactive and reports a sanitized failure

### Requirement: Repository dependencies are ordered
A package SHALL be able to depend on repository and signing-key resources; cache refresh SHALL occur after repository changes and before dependent package resolution.

#### Scenario: New repository supplies package
<!-- verification-id: OS-PRM-014 -->
- **WHEN** a package depends on a new key and repository
- **THEN** the agent installs the verified key, activates the repository, refreshes metadata once, and then resolves the package

### Requirement: Credentials are referenced and redacted
Repository credentials and tokens SHALL enter desired state only through secret references and SHALL never appear in generated source files when a provider-supported credential file or helper is available, logs, drift reports, or command diagnostics.

#### Scenario: Authenticated repository probe fails
<!-- verification-id: OS-PRM-015 -->
- **WHEN** a repository requiring credentials cannot be reached
- **THEN** the error identifies the repository and failure class without including credentials or authenticated URLs

### Requirement: Initial and extended provider gates are explicit
Before this umbrella closes, exact native-package convergence SHALL be qualified for APT on Debian 12 amd64 and Ubuntu 24.04 amd64 and for Pacman plus a truthful AUR/`yay` provider on the pinned Arch 2026-07-06 amd64 release. APT repository/key behavior on Debian and Ubuntu and Pacman repository/signing-trust behavior on Arch SHALL be qualified in the same closeout. Every advertised row SHALL pass provider-contract compliant, drifted, Apply, second-Check, absence, exact-version, failure, lock, redaction, and preservation evidence applicable to that provider in the named real environment. A row SHALL remain unadvertised until its complete provider-contract revision and distribution evidence passes. Fedora/RHEL, DNF4/DNF5, RPM repositories, image-based RPM systems, APK, Zypper, Snap, and other immutable-image providers SHALL remain unadvertised until future OpenSpec changes define and test their complete matrices.

#### Scenario: Unreleased provider is authored
<!-- verification-id: OS-PRM-016 -->
- **WHEN** configuration selects a provider not present in the target agent capability matrix
- **THEN** release validation fails rather than accepting a future placeholder

#### Scenario: Arch repository supplies a managed package
<!-- verification-id: OS-PRM-018 -->
- **WHEN** a package on the pinned Arch release depends on a Remotr-owned repository and declared signing trust
- **THEN** the provider verifies and activates that trust and repository, refreshes metadata once, converges the package, preserves unrelated Pacman configuration, and reports a compliant second Check

#### Scenario: A passing row lacks executable evidence
<!-- verification-id: OS-PRM-023 -->
- **WHEN** a provider matrix row is marked `passing` without selectors proving its complete required contract in the exact distribution, release, architecture, backend, contract revision, and environment
- **THEN** matrix validation fails and the capability is not advertised

### Requirement: Package outcomes report activation needs
Package Apply SHALL report whether service activation or reboot is required without performing an implicit reboot.

#### Scenario: Kernel package requires reboot
<!-- verification-id: OS-PRM-017 -->
- **WHEN** a successful package transaction indicates a reboot is required
- **THEN** the result records `reboot-required` and leaves reboot coordination to the reboot capability

### Requirement: AUR package builds are isolated and reviewable
The AUR provider SHALL resolve and build packages as a declared unprivileged build identity in a bounded workspace, SHALL use only typed package intent rather than caller-supplied commands or build flags, SHALL record sanitized source/package identity and the produced artifact digest, and SHALL perform privileged installation only through the Pacman package boundary under the shared package lock. Transient build material SHALL be cleaned according to the transaction cleanup policy.

#### Scenario: AUR package converges
<!-- verification-id: OS-PRM-024 -->
- **WHEN** a qualifying Arch endpoint selects `yay` for an available package and all declared prerequisites are present
- **THEN** the provider builds as the unprivileged identity, installs the identified artifact through Pacman under the package lock, and reports a compliant second Check

#### Scenario: Exact AUR version is unavailable
<!-- verification-id: OS-PRM-025 -->
- **WHEN** an AUR package declares an exact version that the controlled source cannot resolve
- **THEN** Apply fails without installing another version, leaves the native package database consistent, and reports a sanitized unavailable-version reason

#### Scenario: Arbitrary AUR build input is authored
<!-- verification-id: OS-PRM-026 -->
- **WHEN** desired state attempts to supply a shell command, PKGBUILD body, or arbitrary build flag to the AUR provider
- **THEN** schema validation rejects the undeclared input before release

### Requirement: Exact-version installation uses the resolved native artifact
When APT or Pacman package intent declares an exact version, the provider SHALL install the repository-resolved artifact for that exact version. Querying availability followed by an unversioned installation that can select a different version SHALL NOT satisfy the contract. Provider-native comparison SHALL determine upgrade and downgrade direction, and a prohibited or unavailable transition SHALL leave the package database consistent.

#### Scenario: Pacman resolves an older permitted version
<!-- verification-id: OS-PRM-027 -->
- **WHEN** the installed Pacman package is newer, the declared exact version is available from configured repositories, and downgrade is permitted
- **THEN** Apply installs the resolved declared version and the second Check reports compliance

#### Scenario: Repository selection changes after resolution
<!-- verification-id: OS-PRM-028 -->
- **WHEN** the native install boundary cannot prove it is installing the exact version that was resolved
- **THEN** Apply fails closed rather than issuing an unversioned transaction
