# Package provider qualification matrix

Remotr advertises package and repository behavior only for an exact qualifying
row. A row is identified by capability ID, provider, distribution, release,
architecture, backend, provider-contract revision, and evidence environment.
Family names, rolling aliases such as `latest`, and evidence from a neighboring
distribution do not match a row.

## Qualifying rows

All rows in this first package-provider contract use revision `v1` and the
`container` evidence environment.

| Distribution | Release | Architecture | Backend | Capability boundary |
| --- | --- | --- | --- | --- |
| Debian | 12 | amd64 | `apt` | Native packages, APT repositories, and scoped APT signing keys |
| Ubuntu | 24.04 | amd64 | `apt` | Native packages, APT repositories, and scoped APT signing keys |
| Arch | 2026-07-06 | amd64 | `pacman` | Native repository packages, Pacman repositories, and provider-native signing trust |
| Arch | 2026-07-06 | amd64 | `yay` | AUR package resolution and unprivileged builds followed by exact-artifact Pacman installation |

The Debian and Ubuntu rows qualify independently. The Pacman and `yay` rows
also qualify independently even though they share the Pacman database lock and
privileged installation boundary.

## Supported package intent

Every qualifying package backend supports explicit `present` and `absent`
lifecycle, optional exact-version convergence, upgrade and downgrade policy,
noninteractive execution, bounded diagnostics, dependency-removal policy where
the native backend supports it, and observable activation results.

Backend-specific behavior is deliberately not inferred:

- APT additionally supports `purged` lifecycle and managed hold/unhold state.
- Pacman rejects purge and hold intent. Exact versions are installed only from
  a repository-resolved artifact; an unversioned `pacman -S <name>` does not
  satisfy exact-version intent.
- `yay` accepts only typed package intent. It rejects shell commands, PKGBUILD
  bodies, arbitrary build flags, purge, and hold intent. It builds as the
  declared unprivileged identity and installs only the identified artifact
  through the Pacman boundary.
- No selected backend falls through to a different provider when its executable,
  build identity, platform, or runtime prerequisites are unavailable.

Repository and trust capabilities are separate from package capability. APT
owns deterministic source, preference, credential-helper, and scoped-keyring
paths. Pacman owns deterministic repository fragments, a single managed
`pacman.conf` include boundary, and only the declared provider-native trust
operation. Both preserve unrelated native configuration.

## Required evidence

A row remains `untested` and unadvertised until every applicable layer below
has an exact executable selector in `test/provider-matrix.yaml` and
`test/traceability.yaml`:

| Layer | Required proof |
| --- | --- |
| Configuration | Provider-aware validation, malformed and unsupported intent, parser round trip, and bounded fuzz properties |
| Provider contract | Compliant, drifted, Apply, second Check, absence, exact version, blocked transition, failure, and cleanup outcomes |
| Process boundary | Exact argv, no shell execution, sanitized environment, bounded stdout/stderr, effective user where applicable, and context cancellation |
| Coordination | Shared native lock domain, bounded acquisition, trust/repository/package ordering, and one coalesced metadata refresh |
| Secret and preservation | Secret canaries, complete fingerprint validation, atomic activation or recovery, and preservation of unrelated configuration and trust |
| Real provider | The actual native package manager and trust/configuration databases in the exact pinned image against deterministic signed fixtures |
| Critical mutation | Provider selection, exact-version enforcement, downgrade policy, fingerprint verification, AUR privilege separation, ordering, and advertisement gating |

The provider-matrix gate rejects a `passing` row whose selectors do not prove
the complete `v1` contract for that exact support key. Capability documents are
derived only from matching passing rows.

## Deferred providers

DNF4, DNF5, RPM repositories and image-based RPM systems, APK, Zypper, Snap,
and other immutable-image providers remain absent from advertised package
capabilities. Dormant enum values, code stubs, or executable discovery are not
qualification evidence.
