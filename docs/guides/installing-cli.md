# Installing the operator CLI

The `remotr` operator CLI is distributed as semver-tagged releases built with [GoReleaser](https://goreleaser.com/).

## From GitHub Releases

1. Open [GitHub Releases](https://github.com/DavidHoenisch/remotr/releases).
2. Download the archive for your platform, for example:
   - Linux amd64: `remotr_1.0.0_linux_amd64.tar.gz`
   - macOS arm64: `remotr_1.0.0_darwin_arm64.tar.gz`
   - Windows amd64: `remotr_1.0.0_windows_amd64.zip`
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
| macOS | amd64, arm64 |
| Windows | amd64 |

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

Global flags may appear **before** the subcommand:

```bash
remotr --server-url https://remotr.example.fly.dev endpoint list
remotr config init --server-url https://remotr.example.fly.dev --state-dir ~/.config/remotr/prod --fleet default
remotr config show
remotr git sync
```

`remotr bootstrap` writes the config file automatically after a successful bootstrap.

### Command reference

| Command | Purpose |
|---------|---------|
| `remotr init` | Scaffold configuration repository (`modules/`, `manifest.yaml`, composed artifacts) |
| `remotr bootstrap` | Exchange bootstrap token for operator credentials |
| `remotr enroll token create` | One-time enrollment token |
| `remotr enroll deployment …` / `remotr deployment …` | Reusable deployment tokens (create, list, show, revoke) |
| `remotr endpoint list` / `show` / `remove` | Endpoint inventory |
| `remotr endpoint agent upgrade` | Taint one endpoint for agent upgrade |
| `remotr fleet agent upgrade` | Taint all endpoints in a fleet |
| `remotr git sync` | Trigger server config repo fetch |
| `remotr config compose` / `validate` | Compose manifests into artifacts; validate repo |
| `remotr hub snippet import [entry-id]` | Copy Hub catalog snippet into config repo module (interactive picker when id omitted) |
| `remotr config show` / `path` / `init` | Operator CLI config |
| `remotr upgrade` | Self-upgrade CLI from GitHub Releases |
| `remotr ai setup --agent claude` | Install Remotr AI skill for Claude, Cursor, or Pi |
| `remotr ai upgrade --agent claude` | Update AI skill from GitHub Releases |
| `remotr version` | Print CLI version |

Built-in help: `remotr help`, `remotr endpoint agent upgrade --help`.

See [Operator overview](operator-overview.md) for bootstrap and day-to-day commands (with terminal recordings).

| Workflow | Recording |
|----------|-----------|
| Bootstrap | ![](../assets/demo/bootstrap.gif) |
| Endpoint list | ![](../assets/demo/endpoint-list.gif) |
| Git sync | ![](../assets/demo/git-sync.gif) |

For enrolling Linux endpoints, see [Installing the agent](installing-agent.md).

## Releasing (maintainers)

Releases are created automatically when a semver tag is pushed:

```bash
git tag v1.0.0
git push origin v1.0.0
```

This triggers:

- **`.github/workflows/release.yml`** — GoReleaser builds `remotr` (Linux, macOS, Windows) and `remotr-agent` (Linux amd64/arm64), publishes a GitHub Release with archives and `checksums.txt`
- **`.github/workflows/remotr-server-docker.yml`** — Docker Hub image tags for the same semver (when server paths changed or tag push runs docker workflow)

Test a release locally without publishing:

```bash
make release-snapshot
ls dist/
```

Snapshot binaries are named like `remotr_0.0.1-next-next_linux_amd64.tar.gz`.

Use [Semantic Versioning](https://semver.org/): `vMAJOR.MINOR.PATCH`. Pre-release tags (`v1.0.0-rc.1`) are marked as GitHub pre-releases automatically.
