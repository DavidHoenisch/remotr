# Manifest format reference

**Manifest files** are YAML sources for composing flat `desired.yaml` and `crons.yaml` deployable artifacts. Composition runs locally or in CI via `remotr config compose`; the server serves the generated artifact bytes only.

See [Configuration repository guide](../guides/configuration-repository.md) for layout and workflow.

## Repository paths

| Path | Purpose |
|------|---------|
| `modules/<name>.yaml` | Reusable configuration slice(s) |
| `fleets/<fleet>/manifest.yaml` | Fleet composition source |
| `fleets/<fleet>/desired.yaml` | Generated fleet artifact (commit or CI output) |
| `endpoints/<endpoint-id>/manifest.yaml` | Endpoint composition source |
| `endpoints/<endpoint-id>/desired.yaml` | Generated endpoint override |
| `crons/modules/<name>.yaml` | Reusable cron job module(s) |
| `fleets/<fleet>/crons.manifest.yaml` | Fleet crons composition source |
| `fleets/<fleet>/crons.yaml` | Generated fleet crons artifact |
| `endpoints/<endpoint-id>/crons.manifest.yaml` | Endpoint crons composition source |
| `endpoints/<endpoint-id>/crons.yaml` | Generated endpoint crons override |

## Desired-state manifest (`manifest.yaml`)

```yaml
extends: fleets/engineering/manifest.yaml   # optional
modules:
  - modules/base-packages.yaml
  - modules/sshd-hardening.yaml
overrides:
  - name: base-packages
    packages:
      - name: vim
        present: true
        packageManager: apt
```

| Field | Required | Description |
|-------|----------|-------------|
| `extends` | no | Another manifest path (relative to repository root) whose modules and overrides are included first |
| `modules` | no* | Ordered list of module YAML paths to concatenate |
| `overrides` | no | Replace or patch configuration slices by `name` |

\* At least one of `modules` or `overrides` must be present after resolving `extends`.

## Module files

Each module file uses the same top-level shape as a deployable artifact:

```yaml
configurations:
  - name: ssh-hardening
    targetDistros: [Debian, Ubuntu, Arch]
    files: [...]
```

Configuration `name` values must be unique across all modules in a composed manifest.

## Merge semantics

1. Resolve the `extends` chain depth-first (parent modules and overrides first).
2. Append this manifest's `modules` in order.
3. Apply `overrides` by configuration `name`:
   - Scalar fields (`description`, `targetDistros`, `targetArch`, …) replace when set in the override.
   - Resource lists (`packages`, `files`, `commands`, …) replace entirely when the override sets a non-empty list.

Overrides cannot introduce a new configuration name; add new slices via `modules` instead.

## Crons manifest (`crons.manifest.yaml`)

Same composition model as desired state, but modules contain a top-level `crons:` list and overrides target cron jobs by `name`:

```yaml
extends: fleets/engineering/crons.manifest.yaml
modules:
  - crons/modules/weekly-upgrade.yaml
overrides:
  - name: weekly-system-upgrade
    schedule: "0 3 * * 0"
```

Cron modules may use `use: builtin/...` references; the server still resolves those at sync time after composition.

## Hub snippet import

Copy a catalog snippet into your repository as a module:

```bash
remotr hub snippet import base-packages-debian-arch
remotr hub snippet import ssh-hardening -o modules/sshd-hardening.yaml
```

When run from a source checkout, the CLI auto-detects the bundled `hub/` catalog. Otherwise set `--hub-root` or rely on pinned `sourceCommit` fetch from GitHub.

## Compose commands

```bash
remotr config compose .                         # write all artifacts
remotr config compose . --check                 # CI: fail when artifacts are stale
remotr config compose . --dry-run               # show diffs without writing
remotr config compose . --fleet engineering     # one fleet + extending endpoints
remotr config compose . --fleet engineering --print        # print desired.yaml
remotr config compose . --fleet engineering --stdout crons  # print crons.yaml
remotr config compose . --fleet engineering --stdout all    # print both
remotr config validate .                        # validate artifacts (includes compose check)
remotr config validate . --skip-compose-check   # schema-only validation
```

Recommended CI step:

```bash
remotr config compose . --check
remotr config validate .
```

## Related docs

- [Configuration format reference](configuration-format.md)
- [Configuration repository guide](../guides/configuration-repository.md)
