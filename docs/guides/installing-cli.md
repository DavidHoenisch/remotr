# Installing the operator CLI

The `remotr` operator CLI is distributed as semver-tagged releases built with [GoReleaser](https://goreleaser.com/).

## From GitHub Releases

1. Open [GitHub Releases](https://github.com/DavidHoenisch/remotr/releases).
2. Download the archive for your platform, for example:
   - Linux amd64: `remotr_1.0.0_linux_amd64.tar.gz`
   - Linux arm64: `remotr_1.0.0_linux_arm64.tar.gz`
3. Verify checksums:

```bash
sha256sum -c checksums.txt
```

4. Extract and install:

```bash
tar -xzf remotr_1.0.0_linux_amd64.tar.gz
sudo install -m 0755 remotr /usr/local/bin/remotr
remotr version
```

Expected output (example):

```text
remotr 1.0.0 (abc1234, 2026-06-02T12:00:00Z)
```

## Supported platforms

| OS | Architectures |
|----|---------------|
| Linux | amd64, arm64 |

## Self-upgrade

If `remotr` was installed from GitHub Releases, upgrade in place:

```bash
remotr upgrade --check   # see whether a newer release exists
remotr upgrade           # download latest stable and replace current binary
remotr version
```

Use `--version vX.Y.Z` to install a specific release. The command replaces the binary at the path of the running executable (override with `--install-path`). You may need elevated permissions when installed to `/usr/local/bin`.

Builds from source (`remotr dev`) can still run `remotr upgrade` to switch to the latest release binary.

## AI agent skills

Install the **remotr-agent** skill so Claude Code, Cursor, Pi, or compatible agents can operate fleets through the CLI:

```bash
remotr ai setup --agent claude          # user skill: ~/.claude/skills/remotr-agent
remotr ai setup --agent cursor --scope project   # ./.cursor/skills/remotr-agent
remotr ai setup --agent pi              # user skill: ~/.pi/agent/skills/remotr-agent
remotr ai list
remotr ai upgrade --agent pi            # refresh from latest GitHub release
```

The bundle lives in `ai/remotr-agent/` in this repository (`SKILL.md`, reference docs, helper scripts).

## Build from source

```bash
go build -mod=vendor -o remotr ./cmd/remotr
./remotr version   # prints "remotr dev"
```

## Credentials location

After bootstrap, operator credentials default to `~/.config/remotr/` (override with `REMOTR_OPERATOR_STATE_DIR` or `--state-dir`).

To use credentials stamped by an admin on another machine (without bootstrap), see [Using stamped credentials on a new computer](rbac.md#use-stamped-credentials-on-a-new-computer).

## CLI config file

Repeated flags can live in `~/.config/remotr/config.yaml` (override path with `REMOTR_CONFIG` or `--config`):

```yaml
server_url: https://remotr.example.fly.dev
state_dir: ~/.config/remotr/remotr-example
ca: ~/.config/remotr/remotr-example/ca.crt
fleet: default
```

Precedence: **flags > environment > config file > defaults**.

Global flags may appear before or after the subcommand:

```bash
remotr --server-url https://remotr.example.fly.dev endpoint list
remotr config init --server-url https://remotr.example.fly.dev --state-dir ~/.config/remotr/prod --fleet default
remotr config show
remotr git sync
```

`remotr bootstrap` writes the config file automatically after a successful bootstrap.

### Command map

| Command | Purpose |
|---------|---------|
| `remotr init` | Scaffold configuration repository (`modules/`, `kind: manifest`, etc.) |
| `remotr bootstrap` | Exchange bootstrap token for operator credentials |
| `remotr enroll token create` | One-time enrollment token |
| `remotr enroll deployment …` / `remotr deployment …` | Reusable deployment tokens (create, list, show, revoke) |
| `remotr endpoint list` / `show` / `remove` | Endpoint inventory |
| `remotr endpoint agent upgrade` | Taint one endpoint for agent upgrade |
| `remotr fleet agent upgrade` | Taint all endpoints in a fleet |
| `remotr git sync` | Trigger server config repo fetch |
| `remotr config render` / `validate` / `discover` | Preview composed artifacts; validate repo; list files by kind |
| `remotr hub snippet import [entry-id]` | Copy Hub catalog snippet into config repo module (interactive picker when id omitted) |
| `remotr config show` / `path` / `init` | Operator CLI config |
| `remotr upgrade` | Self-upgrade CLI from GitHub Releases |
| `remotr ai setup --agent claude` | Install Remotr AI skill for Claude, Cursor, or Pi |
| `remotr ai upgrade --agent claude` | Update AI skill from GitHub Releases |
| `remotr version` | Print CLI version |

See the [complete CLI reference](../reference/cli.md) for every endpoint,
fleet, diagnostics, inventory, change-control, secret, RBAC, audit, firewall,
application, and package command with its flags and exit codes.

Built-in help: `remotr help`, `remotr endpoint agent upgrade --help`.

See [Operator overview](operator-overview.md) for bootstrap and day-to-day commands (with terminal recordings).

| Workflow | Recording |
|----------|-----------|
| Bootstrap | ![](../assets/demo/bootstrap.gif) |
| Endpoint list | ![](../assets/demo/endpoint-list.gif) |
| Git sync | ![](../assets/demo/git-sync.gif) |

For enrolling Linux endpoints, see [Installing the agent](installing-agent.md).

## Releasing (maintainers)

### Tagged CLI, agent, and desktop release

CLI and agent releases are created automatically when a semver tag is pushed:

```bash
git tag v1.0.0
git push origin v1.0.0
```

This triggers:

- **`.github/workflows/release.yml`** — builds and lifecycle-smokes the Linux/amd64 Flatpak, then GoReleaser builds `remotr` and `remotr-agent` for Linux (amd64, arm64) and publishes the archives, Flatpak, checked desktop manifest, machine-readable desktop parity inventory, and `checksums.txt`
- **`.github/workflows/remotr-server-docker.yml`** — Docker Hub image tags for the same semver (when server paths changed or tag push runs docker workflow)

GoReleaser uploads the prebuilt
`remotr-desktop_<version>_amd64.flatpak` only after
`make desktop-flatpak-release-check` proves build, install, native launch,
remove, and integrity-manifest evidence. The Flatpak, its manifest, and
`desktop-cli-parity.json` are also included in `checksums.txt`.

Test a release locally without publishing:

```bash
make release-snapshot
ls dist/
```

Snapshot binaries are named like `remotr_0.0.1-next-next_linux_amd64.tar.gz`.

Use [Semantic Versioning](https://semver.org/): `vMAJOR.MINOR.PATCH`. Pre-release tags (`v1.0.0-rc.1`) are marked as GitHub pre-releases automatically.

### Desktop development publication

`.github/workflows/desktop.yml` is a separate, read-only Linux workflow. After
the exact native build, install, launch, remove, and manifest checks pass, it may
upload the Linux/amd64 DEB and `release-manifest.json` as a seven-day Actions
artifact. The unsigned development snapshot is **not attached to the GitHub Release**
created by GoReleaser.

For a manual run, set the `publish_development_snapshot` input to `false` to run
all desktop, parity, and root regression gates without uploading the artifact.
Pull-request runs retain the evidenced upload for reviewer inspection.

No signed Remotr Desktop release output is configured. The Flatpak target is an
explicitly unsigned, release-eligible GitHub Release asset. The repository
still grants no downstream redistribution license, and the package metadata
continues to declare `LicenseRef-proprietary`; publishing the owner-built asset
does not imply permission to redistribute it. The development DEB remains
`releaseEligible: false` and must not be copied into the GitHub Release.

Stopping desktop development publication is distribution-only: disable the
artifact-upload step or run the manual workflow with
`publish_development_snapshot: false`. It does not require a server migration,
an Admin API change, or a database rollback, and it **does not rotate or move Operator credentials**.
Keep the Admin CLI release and recovery path available;
desktop profile settings may be removed separately without deleting the
referenced Operator credential directory.

Disable tagged desktop publication by removing the Flatpak release gate and
the Flatpak/release-manifest `extra_files` entries from `.goreleaser.yaml`
together. This does not require a server migration and does not rotate or move
Operator credentials; the CLI, agent, and server-image release paths remain
available.
