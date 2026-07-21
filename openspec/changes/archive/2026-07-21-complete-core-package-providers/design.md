## Context

The umbrella already defines a provider-neutral package contract and requires real-environment evidence before a provider is advertised. The implementation is uneven:

- APT supports presence/removal, exact versions, upgrade/downgrade policy, holds, cache refresh, and reboot signaling, but its command policy and evidence do not yet prove the complete contract on Debian 12 and Ubuntu 24.04.
- The current Arch applicator is named `aur` in code but executes only Pacman, reports itself as Pacman, and has no truthful `yay` boundary. Configuration validation therefore rejects `packageManager: yay`.
- APT signing-key and repository resources exist. Equivalent Pacman repository and signing-trust resources do not.
- The provider matrix has Debian 12, Ubuntu 24.04, and pinned Arch package/repository rows, but every one is `untested` and points only at a future aggregate target.
- Package transactions mutate shared native databases and trust stores. Provider identity, exact argv, process user, lock behavior, key fingerprints, repository ownership, secret handling, and second-Check convergence are security and safety properties, not incidental implementation details.

This child change completes the already-scoped first provider matrix. It depends on the umbrella execution-contract and capability-delivery work for common locking, redaction, results, and truthful advertisement, but its provider implementations and evidence can proceed in vertical TDD slices.

## Goals / Non-Goals

**Goals:**

- Qualify APT packages on Debian 12 and Ubuntu 24.04 amd64.
- Qualify Pacman packages and a distinct AUR/`yay` provider on the pinned Arch amd64 release.
- Qualify APT repository/key behavior on Debian and Ubuntu and Pacman repository/signing-trust behavior on Arch.
- Prove compliant, drifted, Apply, second-Check, absence, exact-version, policy-blocked, failure, lock, redaction, and preservation behavior at public provider seams.
- Make capability advertisement a mechanically checked consequence of a passing, release-specific evidence row.

**Non-Goals:**

- DNF4, DNF5, RPM repository or signing policy, rpm-ostree, transactional-update, APK, Zypper, Snap, or other package families.
- Cross-architecture support beyond amd64.
- Arbitrary shell-based package installation, arbitrary PKGBUILD text in desired state, or implicit provider fallback.
- Automatic major operating-system upgrades or implicit endpoint reboot.
- Treating Flatpak, PWA, or Remotr catalog packages as substitutes for the native provider rows in this change.

## Decisions

### 1. The first support matrix is fixed and release-specific

The qualifying rows are:

| Distribution | Release | Architecture | Package backends | Repository backend |
| --- | --- | --- | --- | --- |
| Debian | 12 | amd64 | APT | APT |
| Ubuntu | 24.04 | amd64 | APT | APT |
| Arch | 2026-07-06 pinned image | amd64 | Pacman and AUR/`yay` | Pacman |

Each backend receives its own matrix row and selectors. A passing APT row on Debian does not imply Ubuntu support; a passing Pacman row does not imply AUR support. Release aliases such as `stable` or `latest` are not support keys.

Alternative considered: advertise provider families by distro family or rolling distribution name. Rejected because native tooling and repository behavior can change across releases, and a rolling `latest` image makes evidence irreproducible.

### 2. Provider identity and package intent remain explicit

APT, Pacman, and AUR/`yay` are separate registered providers with separate capability declarations. The normalized package intent contains lifecycle, exact version when declared, upgrade/downgrade permissions, hold state when supported, cache policy, dependency-removal policy, noninteractive policy, architecture, dependencies, notifications, and lock domains.

Validation rejects a field/provider combination that the selected provider cannot honor. In particular, APT `purged` and holds remain APT capabilities; Pacman and AUR do not silently reinterpret them. No provider falls through to another executable merely because both operate on the same native package database.

Alternative considered: retain one Arch provider that selects Pacman or `yay` internally. Rejected because the caller, capability matrix, evidence row, process user, and failure behavior would no longer identify the code path that actually ran.

### 3. Native packages converge only through repository-resolvable versions

APT and Pacman compare installed and requested versions using provider-native comparison. When an exact version is declared, Apply resolves that version through configured repositories and performs an upgrade or permitted downgrade. The command is always argv-based and noninteractive, with a sanitized environment and bounded diagnostic output. Unavailable versions and prohibited transitions return structured failures without corrupting the package database.

Pacman exact-version installation must name an artifact or repository target that actually selects the verified requested version; querying a requested version and then issuing an unversioned `pacman -S <name>` is insufficient. Controlled signed fixture repositories provide at least two known versions so upgrade, downgrade, and unavailable-version outcomes have independently known expected results.

Alternative considered: use the native package cache for downgrades. Rejected as the qualifying contract because cache contents are endpoint-local, mutable, and do not make the desired version reproducibly resolvable.

### 4. AUR builds are isolated from root installation

The AUR provider uses `yay` only as a dedicated unprivileged build user with a bounded working/cache directory. Desired state identifies the package and optional exact version; it never embeds shell commands, PKGBUILD content, or arbitrary build flags. The provider:

1. validates that `yay`, the declared build user, and required build tooling are present;
2. resolves the package metadata and requested version noninteractively;
3. builds as the unprivileged user in the bounded workspace;
4. records sanitized source/package identity and the produced package artifact digest;
5. hands the resulting artifact to the privileged Pacman install boundary under the shared package lock; and
6. removes transient build material according to the transaction cleanup policy.

If those prerequisites or the requested version are unavailable, the provider returns `unsupported` or a structured provider failure as appropriate; it never routes the request through the Pacman repository provider and calls it AUR support. Exact argv and effective user are asserted at the process boundary. The controlled Arch fixture uses a local deterministic AUR-compatible source/package fixture rather than trusting the live AUR network in CI.

Alternative considered: execute `yay` as root. Rejected because AUR build scripts are untrusted build inputs and `yay` itself refuses or discourages that execution model.

### 5. All package mutations share one engine-scoped lock domain

APT package, Pacman package, AUR build/install, signing-trust mutation, repository activation, and metadata refresh participate in a provider-aware package transaction coordinator. APT rows use the Debian package-manager lock domain; Pacman, AUR, Arch repository, and Arch signing-trust rows share the Pacman database/keyring domain. Context cancellation and bounded lock timeouts produce typed failures; the implementation does not sleep or spin indefinitely.

Repository/key dependencies are executed before dependent package resolution. Multiple changed resources in one run coalesce metadata refresh to one successful refresh per native backend after the last relevant repository mutation and before the first dependent package operation.

Alternative considered: rely only on native command locks. Rejected because it would permit Remotr-owned resources to race each other, make ordering nondeterministic, and provide poor bounded failure semantics.

### 6. Repository and signing trust use provider-specific resources with narrow ownership

APT keeps separate signing-key and repository resources. A key is dearmored into a Remotr-owned scoped keyring only after its normalized fingerprint matches. A repository owns only its deterministic source, optional preference, and provider-supported credential fragment. Removal preserves all unrelated sources and trust.

Arch adds separate Pacman signing-key and repository resource kinds rather than extending APT-shaped resources with conditionals:

- A signing-key resource imports into the provider-native Pacman keyring only after exact fingerprint verification and applies only the declared local trust operation.
- A repository resource owns one canonical fragment under a Remotr directory and one idempotently managed include boundary in `pacman.conf`. It declares repository name, server URLs, architecture policy, signature level, and signing-key dependency using typed fields.
- Check parses the effective Pacman configuration and reports safe drift. Apply validates a staged configuration with provider-native tooling before atomically activating owned files. Removal deletes only owned fragments and removes the include boundary only when no managed fragment requires it.

Credentials are references resolved at execution time and are never written into source URLs, desired-state reports, argv, or errors when the provider supports a credential file or helper.

Alternative considered: generic line editing of `/etc/pacman.conf`. Rejected because it cannot establish precise ownership, preserve unrelated configuration reliably, or validate a complete staged result.

### 7. Provider evidence uses controlled signed repositories in real images

Unit tests at process boundaries establish exact argv, environment, process user, redaction, lock, comparison, and error mapping. Provider-contract adapters then exercise compliant, drifted, Apply, second Check, absence, and failure cases. Real provider containers run the actual native package manager against local deterministic signed repositories:

- Debian 12 and Ubuntu 24.04 fixtures expose signed APT metadata, multiple package versions, a mismatched-key case, and unrelated source configuration to preserve.
- The pinned Arch fixture exposes signed Pacman metadata, multiple versions, an unknown/mismatched key case, an unrelated Pacman configuration block, and the controlled AUR-compatible fixture.

Passing rows contain precise selectors for the evidence that actually ran. The aggregate matrix target verifies all required selectors and fails if a row says `passing` without executable evidence. Network access to public distro or AUR infrastructure is not part of the deterministic acceptance condition.

Alternative considered: fake runner tests alone. Rejected because they cannot prove native version comparison, repository metadata, keyring, lock, or package-database behavior.

### 8. Advertisement follows evidence and complete contract revision

The provider registry emits a release-specific capability only when the matching distribution, release, architecture, backend, contract revision, and required environment rows are passing. A package capability identifies lifecycle and policy features, not only the executable name. Repository and signing-trust capabilities are advertised independently.

The capability-compatible delivery change consumes those declarations. Configuration targeting an unsupported or incomplete row is rejected before release; the agent also fails closed if local discovery does not match the selected provider. DNF/RPM and all other excluded families remain absent from advertised capabilities even if dormant code or enum values exist.

## Risks / Trade-offs

- **[AUR execution runs third-party build logic]** → Keep builds unprivileged and isolated, prohibit arbitrary desired-state commands/flags, use deterministic fixtures for acceptance, record source/artifact identity, and keep privileged installation behind the Pacman boundary.
- **[The pinned Arch release will age]** → Treat the exact release as an evidence key and require an explicit matrix/evidence update before moving the pin; do not silently map newer images to the passing row.
- **[Local signed repositories add test-fixture complexity]** → Generate minimal deterministic packages and metadata from reviewed fixture definitions, keep signing material test-only, and validate fixture checksums in the harness.
- **[Package-manager semantics differ]** → Reject unsupported intent during validation and publish granular capability fields rather than inventing lossy provider-neutral behavior.
- **[Repository edits can strand package resolution]** → Stage and validate configuration, verify trust before activation, atomically replace only owned files, retain recovery metadata, and preserve unrelated configuration in negative tests.
- **[Package locks can delay a run]** → Use one deterministic queue with context-aware bounded acquisition and structured timeout reporting; test it without wall-clock sleeps.
- **[Live repositories are mutable or unavailable]** → Use local deterministic signed repositories for acceptance and reserve optional live smoke tests for non-gating diagnostics.

## Migration Plan

1. Add or refine typed schema and validation while continuing to reject `yay` and new Arch repository resources until their providers and evidence are complete.
2. Split the current Arch package implementation into a truthfully named Pacman provider and a separate AUR provider boundary; preserve existing Pacman-authored configuration behavior.
3. Complete APT and Pacman lifecycle/version/policy behavior one red-green provider-contract slice at a time, then add real-image evidence.
4. Add Arch signing-key and repository resources, parser/composition/dependency validation, applicators, preservation tests, and real-image evidence.
5. Add the controlled signed repository/AUR fixtures and replace aggregate `untested` selectors with exact executable selectors.
6. Change required matrix rows to `passing` only after their complete evidence succeeds, then expose their granular capabilities through the capability document.
7. Remove the temporary `yay` authoring rejection only when the AUR provider row is passing; retain explicit validation failures for all deferred providers.
8. Run focused provider evidence, the complete provider matrix, configuration composition/validation, authenticated compatibility tests, and `make test` before closing the child.

Rollback disables the affected capability advertisement and restores authoring rejection for the row before reverting provider code. Existing Remotr-owned repository fragments remain identifiable and can be removed through their resource lifecycle; rollback must not delete unrelated native configuration or trust.

## Open Questions

None. The supported releases, provider boundaries, trust ownership, AUR isolation model, deterministic evidence strategy, and deferrals are fixed by this change.
