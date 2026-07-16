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

No macOS or Windows job, installer, metadata bundle, release asset, or support
claim exists. No Linux arm64, RPM, Flatpak, AppImage, or other target is
advertised. A target can be added only with exact native build, install, launch,
remove, and release-manifest evidence.

No signed release output is configured. The current DEB and its
`release-manifest.json` are unsigned development snapshots with
`releaseEligible: false`; they are not signed production releases. The manifest
binds the advertised file name, size, SHA-256, embedded/package version,
OS/architecture/format, signing classification, and four lifecycle results.

The Linux runtime requires GTK 3 and WebKitGTK 4.1 or 4.0. The current native
Linux/amd64 evidence uses WebKitGTK 4.1 and Wails' `webkit2_41` build tag.

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

Current inventory: `59` implemented, `12` planned, and `1` reviewed not applicable; the published parity claim is `partial`.

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
