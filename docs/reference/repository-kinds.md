# Repository file kinds

A Remotr configuration repository contains four composable YAML source kinds
and one optional metadata document. The `kind` at the top of a file determines
which parser and composition rules apply; it is not the same as a resource
`kind` inside a schema-1 module.

## At a glance

| File kind | Normal location | Selected by | Produces |
| --- | --- | --- | --- |
| `manifest` | `fleets/<fleet>/manifest.yaml`, `endpoints/<id>/manifest.yaml` | Server path convention | Composition entry point |
| `module` | `modules/**/*.yaml` | Manifest `modules:` | Desired-state configuration slices |
| `application` | `applications/**/*.yaml` | Manifest `applications:` | Package resources in the composed desired artifact |
| `crons` | `crons/**/*.yaml` | Manifest `crons:` | Server-dispatched scheduled jobs |
| `remotr-config-repo` | `remotr.yaml` | Operator tooling only | Human/tool metadata; never sent to agents |

`desired.yaml` and `crons.yaml` are composed artifacts, not source kinds. Do
not commit them.

## `kind: manifest`

A manifest selects source files for one fleet or one endpoint override:

```yaml
kind: manifest
extends: fleets/base/manifest.yaml
modules:
  - modules/linux-base.yaml
applications:
  - applications/pwa/chat.yaml
crons:
  - crons/weekly-maintenance.yaml
```

`extends` is optional and resolves a parent manifest first. Lists are then
appended in order. Endpoint manifests replace the fleet artifact at serving
time; use `extends: fleets/<fleet>/manifest.yaml` when the endpoint should keep
the fleet baseline.

See [Manifest format](manifest-format.md) for overrides, directory references,
and merge behavior.

## `kind: module`

A module is the canonical schema-1 authoring unit:

```yaml
kind: module
schemaVersion: 1
configurations:
  - name: base
    targetDistros: [Debian, Ubuntu, Arch]
    resources:
      - kind: package
        name: curl
        lifecycle: present
      - kind: file
        name: motd
        path: /etc/motd
        content: |
          Managed by Remotr
```

Important rules:

- `schemaVersion: 1` is required for new modules;
- configuration names must be unique after composition;
- resource names must be unique across all kinds inside one configuration;
- a resource address is `<configuration>/<resource-name>`;
- `targetDistros` and `targetArch` filter the slice on the endpoint;
- `dependsOn` uses complete resource addresses.

Legacy unversioned modules with plural collections remain readable during the
schema-0 compatibility window. Do not use them for new configuration.

See [Configuration format](configuration-format.md) and [Resource
kinds](resource-kinds.md).

## `kind: application`

An application source is a catalog-oriented package declaration. Its compact
package fields intentionally retain the application-file shape; it is not a
schema-1 `resources` list.

```yaml
kind: application
name: chat
present: true
packageManager: pwa
pwaURL: https://chat.example.internal
pwaTitle: Company Chat
```

Bundle form:

```yaml
kind: application
packages:
  - name: internal/diagnostics
    present: true
    packageManager: remotr
    version: 2.4.1
  - name: org.gnome.Calculator
    present: true
    packageManager: flatpak
```

Fleet manifests may reference an explicit path, a path under
`applications/`, or a unique basename. Ambiguous basenames fail validation.

See [Applications format](applications-format.md).

## `kind: crons`

A crons source defines server-dispatched work. These jobs run only when the
server marks a schedule due and the endpoint checks in; they do not install
native cron entries.

```yaml
kind: crons
crons:
  - name: weekly-upgrade
    schedule: "0 2 * * 0"
    timezone: UTC
    targetDistros: [Debian, Ubuntu]
    commands:
      - name: update-index
        apply: [apt-get, update]
      - name: upgrade
        dependsOn: [weekly-upgrade/update-index]
        apply: [apt-get, upgrade, -y]
```

Cron job resource collections currently use their job-specific plural shape,
not a schema-1 `resources` list. For an OS-native schedule that remains active
while the endpoint is offline, use the `endpointSchedule` desired-state
resource instead.

See [Crons format](crons-format.md).

## `kind: remotr-config-repo`

`remotr init` writes optional operator metadata to `remotr.yaml`:

```yaml
version: 1
kind: remotr-config-repo
defaultFleet: workstations
fleets:
  - name: workstations
    remediationPolicy: report
paths:
  fleetManifest: fleets/<fleet>/ (one kind: manifest)
  endpointOverride: endpoints/<endpoint-id>/ (optional kind: manifest)
```

The server does not use this file to choose the active remediation policy or
compose an artifact. Treat it as repository documentation and tooling hints.
The authoritative fleet policy is stored in the server registry.

## File discovery and path safety

References are repository-relative. Absolute paths and paths that escape the
repository root are rejected. Directory references recursively collect files
of the expected kind and use deterministic ordering. A file with the wrong
kind does not silently change roles.

Use:

```bash
remotr config discover --fleet workstations .
remotr config validate .
remotr config render --fleet workstations .
```

`discover` shows what was selected, `validate` checks every reference and
composition rule, and `render` previews the artifact without writing generated
files into the repository.
