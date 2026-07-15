# Applications format reference

**Application files** (`kind: application`) live in the shared top-level `applications/` catalog. Fleet manifests reference apps by short name or path — definitions are never copied per fleet. The server composes application packages into the fleet desired artifact at release ref advance.

See [Configuration repository guide](../guides/configuration-repository.md) for layout and workflow.

## Shared catalog layout

Organize files under `applications/` however you like. Fleet manifests list apps in the `applications:` field:

| Reference style | Example | Resolves to |
|-----------------|---------|-------------|
| Explicit repo path | `applications/pwa/microsoft/teams.yaml` | That file |
| Path under `applications/` | `pwa/microsoft/teams` | `applications/pwa/microsoft/teams.yaml` |
| Basename crawl | `teams` | Unique `**/teams.yaml` under `applications/` |

If a basename matches more than one file, validation fails with an ambiguity error — use an explicit path.

```text
remotr-config/
├── applications/
│   ├── slack.yaml
│   └── pwa/
│       └── microsoft/
│           ├── teams.yaml
│           └── outlook.yaml
└── fleets/engineering/
    └── manifest.yaml    # applications: [slack, teams]
```

Define each app once. Fleet manifests only **select** apps; fleet-specific pins use manifest `overrides` on package names when needed.

## Application file format (`kind: application`)

Each file describes one or more compact package entries. This repository-file
shape intentionally uses `present: true/false`; it is translated during
composition. Canonical `kind: module` resources instead use
`lifecycle: present/absent`. Do not put `schemaVersion: 1` or a `resources:`
list in an application file.

The remaining provider fields follow [configuration format — Package
resources](configuration-format.md#package-resources).

### Single app per file (preferred)

```yaml
kind: application
name: slack
present: true
packageManager: pwa
pwaURL: https://app.slack.com/client
pwaTitle: Slack
```

### Bundle file (many apps in one file)

```yaml
kind: application
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

When a file sets `targetDistros`, `targetArch`, or a `configuration:` block, composition emits a **separate configuration slice** instead of adding to the shared `applications` slice:

```yaml
kind: application
targetDistros: [Debian, Ubuntu]
name: org.gnome.Calculator
present: true
packageManager: flatpak
```

Package `name` values must be unique across all applications selected by a fleet manifest.

## Link from fleet manifest

List apps inline on the fleet manifest:

```yaml
kind: manifest
modules:
  - modules/base-packages.yaml
applications:
  - slack
  - applications/pwa/microsoft/teams.yaml
```

Endpoint manifests inherit fleet `applications` by extending the fleet
manifest. Their own entries append after the parent's entries. The resulting
endpoint artifact replaces the fleet artifact at serving time. Without
`extends`, only the endpoint manifest's selections are composed.

## Compose output

Default mode produces a configuration slice named `applications` in the composed desired artifact:

```yaml
configurations:
  - name: base-packages
    resources: [...]
  - name: applications
    description: Composed from applications manifest
    resources:
      - kind: package
        name: slack
        lifecycle: present
        packageManager: pwa
        ...
```

The composer emits canonical resource output when the selected module set is
schema 1. The application source itself remains the compact catalog format
shown above.

Preview with:

```bash
remotr config render --fleet engineering
remotr config validate .
```

## Related docs

- [Manifest format reference](manifest-format.md)
- [Configuration format reference](configuration-format.md)
- [Custom app packages](../guides/custom-app-packages.md)
- [Configuration repository guide](../guides/configuration-repository.md)
