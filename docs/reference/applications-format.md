# Applications format reference

**Application files** live in the shared top-level `applications/` catalog. Fleets reference apps by short name or path — definitions are never copied per fleet. Composition runs locally or in CI via `remotr config compose`; output folds into `desired.yaml`.

See [Configuration repository guide](../guides/configuration-repository.md) for layout and workflow.

## Shared catalog layout

Organize files under `applications/` however you like — no required subfolders. Compose resolves references in three ways:

| Reference style | Example | Resolves to |
|-----------------|---------|-------------|
| Explicit repo path | `applications/pwa/microsoft/teams.yaml` | That file |
| Path under `applications/` | `pwa/microsoft/teams` | `applications/pwa/microsoft/teams.yaml` |
| Basename crawl | `teams` | Unique `**/teams.yaml` under `applications/` |

If a basename matches more than one file, compose fails with an ambiguity error — use an explicit path.

```text
remotr-config/
├── applications/
│   ├── manifest.yaml              # optional repo-wide baseline (not an app module)
│   ├── slack.yaml
│   ├── pwa/
│   │   └── microsoft/
│   │       ├── teams.yaml
│   │       └── outlook.yaml
│   └── remotr/
│       └── internal-mycli.yaml
├── fleets/engineering/
│   └── applications.manifest.yaml
└── fleets/sales/
    └── applications.manifest.yaml
```

Define each app once. Fleet manifests only **select** apps and apply fleet-specific overrides.

## Application file formats

Each file describes one or more `packages` entries (same fields as [configuration format — Packages](configuration-format.md#packages)).

### Single app per file (preferred)

```yaml
# applications/slack.yaml
name: slack
present: true
packageManager: pwa
pwaURL: https://app.slack.com/client
pwaTitle: Slack
```

### Bundle file (many apps in one file)

```yaml
# applications/design-suite.yaml
packages:
  - name: slack
    present: true
    packageManager: pwa
    pwaURL: https://app.slack.com/client
  - name: internal/design-cli
    present: true
    packageManager: remotr
    version: "2.1.0"
```

Legacy paths `applications/modules/` and `applications/bundles/` still work when referenced explicitly or found by basename crawl.

### Targeted app (dedicated configuration slice)

When a file sets `targetDistros`, `targetArch`, or a `configuration:` block, compose emits a **separate configuration slice** instead of adding to the shared `applications` slice:

```yaml
# applications/gnome-calculator.yaml
targetDistros: [Debian, Ubuntu]
name: org.gnome.Calculator
present: true
packageManager: flatpak
```

Package `name` values must be unique across all application modules selected by a fleet manifest.

## Repo-wide baseline (`applications/manifest.yaml`)

List apps every fleet should inherit, then extend from fleet manifests:

```yaml
# applications/manifest.yaml
modules:
  - slack
  - internal-mycli
```

```yaml
# fleets/engineering/applications.manifest.yaml
extends: applications/manifest.yaml
modules:
  - design-suite          # engineering-only additions
overrides:
  - name: internal/mycli
    version: "1.5.0"       # fleet-specific pin
```

```yaml
# fleets/sales/applications.manifest.yaml
extends: applications/manifest.yaml
# inherits slack + internal-mycli; no extra modules
```

## Fleet applications manifest (`applications.manifest.yaml`)

Same composition model as desired state and crons:

| Field | Description |
|-------|-------------|
| `extends` | Another applications manifest (often `applications/manifest.yaml`) |
| `modules` | App references: explicit paths, paths under `applications/`, or unique basenames |
| `overrides` | Patch packages by **package name** |
| `mode` | Empty (default): shared `applications` slice; `per-module`: one slice per app |

Short names and partial paths are resolved by walking the `applications/` tree. Basename lookup skips `manifest.yaml` (the repo-wide applications manifest).

Endpoint `applications.manifest.yaml` **replaces** the fleet file when present. When an endpoint manifest extends a fleet and has no local applications source, fleet applications are inherited.

## Link from `manifest.yaml`

Reference a fleet applications manifest:

```yaml
modules:
  - modules/base-packages.yaml
applications: fleets/engineering/applications.manifest.yaml
```

Or select shared catalog apps inline (no separate fleet applications file):

```yaml
modules:
  - modules/base-packages.yaml
applications:
  - slack
  - design-suite
```

If `applications:` is omitted but `applications.manifest.yaml` exists beside `manifest.yaml`, compose uses the sibling file automatically.

## Compose output

Default mode produces a configuration slice named `applications` in `desired.yaml`:

```yaml
configurations:
  - name: base-packages
    packages: [...]
  - name: applications
    description: Composed from applications manifest
    packages:
      - name: slack
        packageManager: pwa
        ...
```

```bash
remotr config compose .
remotr config validate .
```

## Related docs

- [Manifest format reference](manifest-format.md)
- [Configuration format reference](configuration-format.md)
- [Custom app packages](../guides/custom-app-packages.md)
- [Configuration repository guide](../guides/configuration-repository.md)
