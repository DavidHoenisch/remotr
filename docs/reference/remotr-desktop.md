# Remotr Desktop support reference

This page records what Remotr Desktop may currently claim. It is a reference,
not an installation promise. Artifact availability is controlled by native
evidence and the checked release manifest.

## Product identity

| Field | Value |
| --- | --- |
| Product | Remotr Desktop |
| Executable | `remotr-desktop` |
| Application ID | `io.github.davidhoenisch.remotr.desktop` |
| Administration API | Existing mTLS Admin API |
| Credential model | References the existing Operator state directory |
| Desired-state boundary | Committed Configuration repository content and server Git sync |

## Platform and artifact support

Desktop support in this change is **Linux only**.

| OS | Architecture | Format | Classification | Signing | Release eligible | Required gate |
| --- | --- | --- | --- | --- | --- | --- |
| Linux | Linux/amd64 | DEB | Seven-day CI development artifact | unsigned | No | `make desktop-release-check` |
| Linux | Linux/amd64 | Flatpak | Tagged GitHub Release asset | unsigned | Yes | `make desktop-flatpak-release-check` |

No macOS or Windows job, installer, metadata bundle, release asset, or support
claim exists. No Linux arm64, RPM, AppImage, or other target is
advertised. A target can be added only with exact native build, install, launch,
remove, and release-manifest evidence.

No signed release output is configured. The DEB is an unsigned development
snapshot with `releaseEligible: false`. The Flatpak is an unsigned tagged
release asset with `releaseEligible: true`. Each checked manifest binds the
advertised file name, size, SHA-256, embedded/package version,
OS/architecture/format, signing classification, and four lifecycle results.

The Linux runtime requires GTK 3 and WebKitGTK 4.1 or 4.0. The current native
Linux/amd64 evidence uses WebKitGTK 4.1 and Wails' `webkit2_41` build tag.

## Publication and distribution rollback

The read-only `.github/workflows/desktop.yml` publishes the seven-day DEB
development artifact. A tag-triggered `.github/workflows/release.yml` builds
and lifecycle-smokes the unsigned Flatpak before GoReleaser attaches it and its
checked manifest, plus the current `desktop-cli-parity.json`, to the GitHub
Release beside the CLI and agent archives. All three desktop files are included
in `checksums.txt`. No server or Operator credential enters either packaging
workflow.

Disable desktop artifact upload for development by setting the manual workflow's
`publish_development_snapshot` input to `false` or by disabling its upload step.
That switch will **leave the server, Admin API, database, and Operator credential directories unchanged**.
It does not require a schema migration, data migration,
credential rotation, or credential relocation. The tagged `remotr` and
`remotr-agent` release continues independently.

Disable the tagged unsigned Flatpak by removing its release gate and the
Flatpak/release-manifest `extra_files` entries from `.goreleaser.yaml` in the
same change. That rollback is distribution-only and leaves the CLI, agent,
server, Admin API, database, and Operator credential directories unchanged.

Removing `~/.config/remotr/desktop-profiles.json` removes only desktop profile
references. It does not remove the Operator credential directory. The Admin CLI
continues to use that credential layout for automation and recovery.

The repository grants no downstream redistribution license, and the Flatpak's
AppStream metadata declares `LicenseRef-proprietary`. The owner-published
Flatpak is explicitly unsigned; a future signed channel still requires a
reviewed signing and distribution policy.

## Profile and credential files

| Path or file | Purpose | Contains secret key material |
| --- | --- | --- |
| `~/.config/remotr/config.yaml` | Standard Operator CLI configuration and Default desktop-profile source | No |
| `~/.config/remotr/desktop-profiles.json` | Owner-only named desktop profile references | No |
| `operator.crt` | Operator client certificate | No private key |
| `operator.key` | Operator client private key | Yes; never enters ordinary frontend state |
| `ca.crt` | Remotr CA trust anchor | No |
| `state.json` | Operator identity metadata | No |

The desktop profile file does not duplicate credential bytes. Removing or
replacing desktop profile settings does not revoke or delete the referenced
Operator credential.

## Admin CLI parity status

The authoritative inventory is
[`desktop-cli-parity.json`](desktop-cli-parity.json). Its `publication` object
contains the `parity_claim`, update date, and drift gate. Each non-hidden Admin
CLI workflow is implemented, planned for a named feature release, or reviewed
not applicable only for interface mechanics.

Current inventory: `59` implemented, `14` planned, and `1` reviewed not applicable; the published parity claim is `partial`.

Global Secret administration is intentionally CLI/Admin-API-only in this
release. The desktop continues to support Fleet- and Endpoint-scoped uploads
and exact-name version inspection, but it does not yet offer global scope
selection or the authorization-filtered logical-Secret collection. Those
desktop workflows are tracked for `global-secrets-desktop`; use `remotr secret
list`, `remotr secret show`, and `remotr secret upload --global` in the
meantime.

Do not infer complete parity from the presence of the desktop package. To
inspect the current counts directly:

```bash
jq '.publication, (.entries | group_by(.status) | map({status: .[0].status, count: length}))' \
  docs/reference/desktop-cli-parity.json
```

The drift gate is:

```bash
go test ./cmd/remotr -run '^TestDesktopCLIParityInventoryMatchesCommandTree$' -count=1
```

Implemented entries must retain passing public-seam selectors. Planned entries
must retain a target release and cannot claim passing selectors. A feature
release must not claim full parity while an applicable entry remains planned.

## Fallback and recovery guarantee

The `remotr` Admin CLI remains supported for interactive terminal use,
automation, scripting, headless operation, and recovery. Publishing or stopping
Remotr Desktop does not migrate the server, change the Admin API, move Operator
credentials, or remove the CLI. See [Use Remotr
Desktop](../guides/use-remotr-desktop.md#cli-fallback-and-recovery) for recovery
commands.
