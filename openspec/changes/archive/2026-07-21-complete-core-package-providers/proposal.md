## Why

The applicator umbrella currently describes package and repository behavior more completely than Remotr can prove: APT and Pacman lack complete exact-version provider evidence, `yay` is not a truthful AUR provider, and Arch repository/signing trust has no qualified path. These are already-scoped core providers, so the umbrella must finish them now rather than leave package management behind stubs or defer it to a later feature wave.

## What Changes

- Complete native package lifecycle, exact-version convergence, downgrade policy, managed holds where supported, bounded lock handling, noninteractive execution, and activation reporting for APT on Debian 12 and Ubuntu 24.04 and Pacman on the pinned Arch release.
- Implement a distinct, truthfully advertised AUR provider for `yay` selections, including a non-root build boundary, exact requested-version behavior, serialized native installation, sanitized diagnostics, and an explicit `unsupported` result when its prerequisites are absent.
- Complete APT repository and scoped signing-key management on Debian and Ubuntu, including Remotr-owned fragments, fingerprint verification, dependency ordering, one metadata refresh, and preservation of unrelated configuration.
- Complete Pacman repository and signing-trust management on Arch using Remotr-owned configuration, fingerprint verification, provider-native keyring behavior, dependency ordering, one metadata refresh, and preservation of unrelated Pacman configuration.
- Qualify every advertised row through provider-contract compliant, drifted, Apply, and second-Check evidence in real Debian 12, Ubuntu 24.04, and pinned Arch environments with exact process-boundary argv assertions.
- Keep DNF4/DNF5, RPM-family repositories, APK, Zypper, Snap, and immutable-image package systems unadvertised until their own OpenSpec changes define and qualify complete provider matrices.
- Close umbrella package-provider tasks only after this change's implementation and evidence are accepted.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `package-and-repository-management`: Refine the umbrella package, AUR, repository, signing-trust, provider-gating, and real-environment qualification requirements for the first supported Debian, Ubuntu, and Arch matrix.

## Impact

- Package and repository resource schemas, validation, provider registry, provider capability documents, execution locks, command runners, activation reporting, and release compatibility requirements.
- APT, Pacman, AUR/`yay`, Debian repository, Ubuntu repository, Arch repository, and provider-native signing-key implementations.
- Controlled signed-package fixtures, Debian 12, Ubuntu 24.04, and pinned Arch provider containers, exact-argv tests, traceability, and capability-advertisement evidence.
- Documentation and configuration examples that currently imply or select package providers without a fully qualified row.
- This child remains linked to the active umbrella and is not archived ahead of the umbrella capability baseline.
