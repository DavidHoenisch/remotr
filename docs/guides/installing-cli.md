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

## Build from source

```bash
go build -mod=vendor -o remotr ./cmd/remotr
./remotr version   # prints "remotr dev"
```

## Credentials location

After bootstrap, operator credentials default to `~/.config/remotr/` (override with `REMOTR_OPERATOR_STATE_DIR` or `--state-dir`).

## CLI config file

Repeated flags can live in `~/.config/remotr/config.yaml` (override path with `REMOTR_CONFIG` or `--config`):

```yaml
server_url: https://remotr.example.fly.dev
state_dir: ~/.config/remotr/remotr-example
ca: ~/.config/remotr/remotr-example/ca.crt
fleet: default
```

Precedence: **flags > environment > config file > defaults**.

```bash
remotr config init --server-url https://remotr.example.fly.dev --state-dir ~/.config/remotr/prod --fleet default
remotr config show
remotr endpoint list
remotr git sync
```

`remotr bootstrap` writes the config file automatically after a successful bootstrap.

See [Operator workflows](operator-workflows.md) for bootstrap and day-to-day commands.

## Releasing (maintainers)

Releases are created automatically when a semver tag is pushed:

```bash
git tag v1.0.0
git push origin v1.0.0
```

This triggers:

- **`.github/workflows/release.yml`** — GoReleaser builds `remotr` for all platforms and publishes a GitHub Release with archives and `checksums.txt`
- **`.github/workflows/remotr-server-docker.yml`** — Docker Hub image tags for the same semver (when server paths changed or tag push runs docker workflow)

Test a release locally without publishing:

```bash
make release-snapshot
ls dist/
```

Snapshot binaries are named like `remotr_0.0.1-next-next_linux_amd64.tar.gz`.

Use [Semantic Versioning](https://semver.org/): `vMAJOR.MINOR.PATCH`. Pre-release tags (`v1.0.0-rc.1`) are marked as GitHub pre-releases automatically.
