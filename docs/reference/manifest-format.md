# Manifest format reference

**Manifest files** (`kind: manifest`) are fleet and endpoint entry points. They list modules, applications, and cron sources; the Remotr server **composes** flat deployable artifacts when the release ref advances and caches them in Postgres. Authors preview the same output locally with `remotr config render`.

See [Configuration repository guide](../guides/configuration-repository.md) for layout and workflow. See [ADR-004](https://github.com/DavidHoenisch/remotr/blob/master/engineering/adr/004-server-side-composition.md) for the server-side composition model.

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
    # Overrides currently use the grouped Configuration shape.
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
schemaVersion: 1
configurations:
  - name: ssh-hardening
    targetDistros: [Debian, Ubuntu, Arch]
    resources:
      - kind: file
        name: sshd-policy
        path: /etc/ssh/sshd_config.d/90-remotr.conf
        content: |
          PermitRootLogin no
```

Configuration `name` values must be unique across all modules in a composed manifest.
New modules use `schemaVersion: 1` and one canonical `resources` list. Legacy
unversioned modules with plural resource collections remain readable during
the compatibility window.

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
The override model currently uses the legacy grouped collection names even
when the selected modules are schema 1. Do not put a canonical `resources:`
list inside `overrides`; use an additional module when you want canonical
schema-1 authoring or additive behavior.

Cron jobs from referenced `kind: crons` files are concatenated (duplicate names fail validation).

## Endpoint override manifest

```yaml
kind: manifest
extends: fleets/engineering/manifest.yaml
modules:
  - modules/designer-extra.yaml
```

An endpoint artifact **replaces** the fleet artifact when served. The example
still contains the fleet state because composition resolves `extends` first,
then appends the endpoint module. Without `extends`, only the endpoint
manifest's selections are present. Compose the full divergent state in Git;
there is no fleet/endpoint merge at sync time.

## Hub snippet import

Copy a catalog source into your repository:

```bash
remotr hub snippet import base-packages-debian-arch
remotr hub snippet import weekly-system-upgrade-builtin
```

Module entries default to `modules/<entry-id>.yaml`; cron entries default to
`crons/<entry-id>.yaml`. Reference the imported path from a fleet manifest.

When run from a source checkout, the CLI auto-detects the bundled `hub/`
catalog. Otherwise set `--hub-root` or use the published catalog.

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
- [ADR-004 — server-side composition](https://github.com/DavidHoenisch/remotr/blob/master/engineering/adr/004-server-side-composition.md)
