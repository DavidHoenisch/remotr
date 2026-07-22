# Capability delivery blocker evidence

This record freezes the sanitized production reproduction that led to
`fix-capability-delivery-blockers`. It contains no enrollment token, resolved
secret, endpoint credential, or production endpoint identifier.

## Public seams and verification IDs

| Concern | Verification IDs | Public seam | Required evidence |
| --- | --- | --- | --- |
| Target normalization and validation parity | OS-AEC-104–107, OS-PRM-030 | `remotr config validate`; Admin Git-sync API | Differential CLI/API cases, malformed and boundary tables, bounded fuzzing, redacted diagnostics |
| Mixed-target requirement projection | OS-AEC-108, OS-UPM-063 | Configuration render/discover; authenticated Sync | Ubuntu/Arch fixture, complete canonical artifact equality, target-specific missing-requirement assertions, benchmark |
| Blocked delivery and acknowledgment | OS-AEC-022–024, OS-AEC-090–091, OS-AEC-109 | Authenticated Sync | Existing/new endpoint cases, no partial artifact, old active digest retained, exact-digest acknowledgment |
| Production capability publication | OS-LPC-023–027, OS-UPM-061–064 | Composed agent execution; authenticated Sync | Frozen catalog validation, production default generator, mismatched-fact negatives, packaged-output parity |
| Ubuntu 26.04 package qualification | OS-PRM-029–030 | Provider contract; configuration CLI | Pinned Ubuntu 26.04 amd64 APT evidence or author-time rejection |
| Capability-blocked upgrade escape | OS-AEC-025, OS-AEC-109–110 | Authenticated Sync; released agent decoder | Approved upgrade eligibility, negative authorization/platform cases, legacy decoding, post-upgrade capability reevaluation |

## Sanitized engineering target inventory

The public regression repository is
`test/config-repos/capability-delivery-blockers`. Its configurations represent
the target shapes from the engineering fleet without production names or
values:

| Normalized target | Applicable configuration classes | Target-only requirements |
| --- | --- | --- |
| portable | universal package applications | `resource:package`; Flatpak, PWA, and Remotr package providers |
| Ubuntu or Debian, any architecture | common POSIX/systemd resources | file, command, download, bootstrap, and systemd resource/provider contracts |
| Ubuntu or Debian, x86 | APT applications | `provider:package/apt` |
| Arch, any architecture | Arch POSIX/systemd and user-session resources | `resource:userFile` plus common contracts |
| Arch, x86 | Pacman applications | `provider:package/pacman` |
| exact Ubuntu, x86 | Ubuntu Pro attachment with `esm-apps` full mode | Ubuntu Pro resource, service, and option contracts |

The source format does not currently declare a distribution release. Exact
release support is therefore established by the endpoint capability document
and the frozen qualification catalog; author-time validation rejects a
distribution/architecture/provider combination when the catalog has no
passing release row for it. It does not invent an endpoint release.

## Reproduced aggregate failure

Before target-aware projection, public `config discover` reports this one
aggregate set for the representative engineering artifact:

```text
provider:init/systemd@1
provider:package/apt@1
provider:package/flatpak@1
provider:package/pacman@1
provider:package/pwa@1
provider:package/remotr@1
provider:ubuntu-pro-option/esm-apps/full@1
provider:ubuntu-pro-service/esm-apps@1
resource:bootstrap@bootstrap-v1
resource:command@command-v1
resource:download@download-v1
resource:file@file-v1
resource:package@package-v1
resource:systemd@systemd-v1
resource:ubuntu-pro@ubuntu-pro-v1
resource:user-file@userFile-v1
schema:1@1
```

The affected Ubuntu 26.04 amd64 agent already supplied schema 1 and
`provider:package/remotr@1`, leaving the observed 15 missing requirements.
Two are categorically irrelevant to that endpoint:
`provider:package/pacman@1` and `resource:user-file@userFile-v1`. The correct
Ubuntu x86 projection retains the complete canonical desired YAML while
excluding those two requirements.

The initial target-aware implementation baseline on the representative fixture
(AMD Ryzen AI 9 HX 370, linux/amd64, Go 1.26.5) is:

```text
BenchmarkRenderMixedTargetArtifactVariants-24       23,971,318 ns/op  24,996,443 B/op  154,154 allocs/op
BenchmarkSelectMixedTargetArtifactVariant-24             5,817 ns/op       2,834 B/op       40 allocs/op
BenchmarkCapabilityVariantSelection400Endpoints-24   3,158,311 ns/op   1,102,223 B/op   14,809 allocs/op
```

These figures are evidence for regression comparison, not universal service
objectives. Variant generation remains bounded by authored target values and
selection does not use endpoint identity.

## Ubuntu 26.04 qualification result

The Ubuntu 26.04 amd64 native-package contracts passed against the pinned OCI
manifest `sha256:7c2884fd32770fc6c173b78e0dc2278a2851d89f5447919edbc45475ac55dd6a`:

- APT package compliant, drifted, Apply, second Check, exact upgrade and
  downgrade, hold, unavailable version, lock, reboot-marker, remove, and purge;
- APT repository/key present, disabled, absent, exact fingerprint rejection,
  scoped authentication, unrelated-file preservation, and composed package
  convergence; and
- file and download convergence, rollback, checksum, redirect, authentication
  redaction, absence, and second Check.

The pinned `cloud-image/ubuntu-26.04` Vagrant box version `20260720.0.0`
passed the complete credential-free Ubuntu Pro contract and its ordinary,
specialized, conflict, fault, rollback, secret-canary, negative-identity, and
cleanup matrices. No live Canonical account or entitlement was used.

The same pinned VM independently qualified the engineering inventory's command,
bootstrap, systemd, Flatpak, and PWA providers. Flatpak publication requires an
observed `flatpak` executable. PWA publication additionally requires an observed
qualified Chromium-family browser backend; implementation presence alone does
not publish either capability. Unsupported or absent runtime providers remain
capability blocked.

Public `config validate`, `config discover`, `config render`, production
capability generation, and authenticated Sync were exercised with the sanitized
mixed Ubuntu/Arch fixture. The Ubuntu 26.04 endpoint received the complete
artifact, including the preserved Arch branch, without Pacman in its matching
requirement set; the exact digest became active only after acknowledgment.

## Released legacy decoder floor

`agentUpgrade` first shipped in tag `v0.1.13` (commit `d5084c37d`). Its Sync
response decoder uses Go's ordinary JSON decoding and therefore ignores later
unknown fields such as `capabilityBlocked`; its run loop processes
`agentUpgrade` even when the response has no artifact. The capability-blocked
success behavior itself first shipped in `v0.6.7` (commit `aa8510eba`).

Accordingly:

- `v0.1.13` and newer released agents are eligible for the compatibility
  fixture that combines `capabilityBlocked` and `agentUpgrade`.
- releases older than `v0.1.13` have no in-band upgrade response contract and
  require the documented out-of-band installer path.
- version metadata never proves runtime provider support; after any upgrade,
  the server waits for a valid current capability document and reevaluates the
  artifact.
- an explicitly requested, approved release can be returned alongside
  capability-blocked metadata; artifact-provider requirements do not suppress
  that upgrade escape path.
