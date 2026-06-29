# Manifest format reference

**Manifest files** (`kind: manifest`) are fleet and endpoint entry points. They list modules, applications, and cron sources; the Remotr server **composes** flat deployable artifacts when the release ref advances and caches them in Postgres. Authors preview the same output locally with `remotr config render`.

See [Configuration repository guide](../guides/configuration-repository.md) for layout and workflow. See [ADR-004](../adr/004-server-side-composition.md) for the server-side composition model.

## Repository paths

| Path | `kind` | Purpose |
|------|--------|---------|
| `modules/<name>.yaml` | `module` | Reusable configuration slice(s) |
| `applications/<path>.yaml` | `application` | Shared application definition |
| `crons/<path>.yaml` | `crons` | Cron job list (inline jobs or `use:` refs) |
| `fleets/<fleet>/` | — | Fleet folder; exactly one `kind: manifest` inside |
| `endpoints/<endpoint-id>/` | — | Optional override; exactly one `kind: manifest` when present |

Do **not** commit `desired.yaml` or `crons.yaml` — those are composed artifacts served to agents, not Git sources.

## Fleet manifest (`kind: manifest`)

```yaml
kind: manifest
extends: fleets/engineering/manifest.yaml   # optional
modules:
  - modules/base-packages.yaml
  - modules/sshd-hardening.yaml
applications:
  - slack
  - applications/pwa/microsoft/teams.yaml
crons:
  - crons/modules/weekly-upgrade.yaml
overrides:
  - name: base-packages
    packages:
      - name: vim
        present: true
        packageManager: apt
```

| Field | Required | Description |
|-------|----------|-------------|
| `kind` | yes | Must be `manifest` |
| `extends` | no | Another manifest path (repo-relative) whose modules, applications, crons, and overrides are included first |
| `modules` | no* | Ordered list of `kind: module` paths or folders |
| `applications` | no | App references: explicit paths, paths under `applications/`, or unique basenames (see [Applications format](applications-format.md)) |
| `crons` | no | `kind: crons` file paths or folders |
| `overrides` | no | Replace or patch configuration slices by `name` |

\* At least one of `modules`, `applications`, `crons`, or `overrides` must contribute content after resolving `extends`.

Folder references: when a list entry is a directory, the tool recursively collects all files of the expected kind under that path.

## Module files (`kind: module`)

```yaml
kind: module
configurations:
  - name: ssh-hardening
    targetDistros: [Debian, Ubuntu, Arch]
    files: [...]
```

Configuration `name` values must be unique across all modules in a composed manifest.

## Cron source files (`kind: crons`)

Cron jobs are listed in dedicated files, referenced from the fleet manifest `crons:` list:

```yaml
kind: crons
crons:
  - use: builtin/system-upgrade-debian
    schedule: "0 0 * * 0"
```

See [Crons format reference](crons-format.md). Builtin `use:` references are resolved by the server at sync time after composition.

## Merge semantics (desired state)

1. Resolve the `extends` chain depth-first (parent modules and overrides first).
2. Append this manifest's `modules` in order.
3. Merge application packages into the composed state.
4. Apply `overrides` by configuration `name`:
   - Scalar fields (`description`, `targetDistros`, `targetArch`, …) replace when set in the override.
   - Resource lists (`packages`, `files`, `commands`, …) replace entirely when the override sets a non-empty list.

Overrides cannot introduce a new configuration name; add new slices via `modules` instead.

Cron jobs from referenced `kind: crons` files are concatenated (duplicate names fail validation).

## Endpoint override manifest

```yaml
kind: manifest
extends: fleets/engineering/manifest.yaml
modules:
  - modules/designer-extra.yaml
```

An endpoint override **replaces** the fleet artifact when present — compose the full divergent state in Git (extends + modules), not a partial delta at sync time.

## Hub snippet import

Copy a catalog snippet into your repository as a module:

```bash
remotr hub snippet import base-packages-debian-arch
remotr hub snippet import ssh-hardening -o modules/sshd-hardening.yaml
```

When run from a source checkout, the CLI auto-detects the bundled `hub/` catalog. Otherwise set `--hub-root` or rely on pinned `sourceCommit` fetch from GitHub.

## CLI commands

```bash
remotr config render .                              # preview all fleets (table output)
remotr config render --fleet engineering            # preview one fleet
remotr config render --endpoint <endpoint-id>       # preview endpoint override
remotr config render --fleet engineering --output /tmp/desired.yaml
remotr config discover --fleet engineering          # list discovered files by kind
remotr config validate .                            # validate kinds, refs, and composition
remotr config validate . --skip-render-check        # schema-only (no composition dry-run)
```

Recommended CI step:

```bash
remotr config validate .
```

## Related docs

- [Applications format reference](applications-format.md)
- [Configuration format reference](configuration-format.md)
- [Configuration repository guide](../guides/configuration-repository.md)
- [ADR-004 — server-side composition](../adr/004-server-side-composition.md)
