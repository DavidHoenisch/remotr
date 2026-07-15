# Operate a configuration repository

The configuration repository is the reviewed Git source for desired state.
The server—not the agent—reads the repository, composes fleet and endpoint
artifacts, and advances a release ref only after composition succeeds.

Agents never clone Git and source repositories do not contain generated
`desired.yaml` or `crons.yaml` files.

## Recommended layout

```text
remotr-config/
├── remotr.yaml                         # optional operator metadata
├── server.env.example                  # deployment example, not a secret file
├── modules/                            # kind: module
│   ├── base-packages.yaml
│   ├── access.yaml
│   └── network.yaml
├── applications/                       # kind: application
│   ├── chat.yaml
│   └── pwa/microsoft/teams.yaml
├── crons/                              # kind: crons
│   └── weekly-maintenance.yaml
├── fleets/
│   ├── workstations/manifest.yaml      # kind: manifest
│   └── servers/manifest.yaml
└── endpoints/
    └── <endpoint-id>/manifest.yaml     # optional kind: manifest
```

The four composable source kinds are `manifest`, `module`, `application`, and
`crons`. `remotr.yaml` has `kind: remotr-config-repo`, but is metadata for
operators and tooling; the server does not serve it to agents or use its
remediation-policy hint as authoritative state.

See [Repository file kinds](../reference/repository-kinds.md) before inventing
new top-level document shapes.

## Scaffold safely

```bash
remotr init --fleet workstations ./remotr-config
```

The destination must be absent or empty. `init` refuses to merge into a
non-empty directory so it cannot silently overwrite an existing repository.

To register the initial fleet and create a one-time enrollment token in the
same operation, use this form instead of running `init` a second time:

```bash
REMOTR_DATABASE_URL='postgres://...' remotr init \
  --fleet workstations \
  --policy report \
  --register-server \
  --enroll \
  --enroll-out /secure/workstations.enroll.token \
  --quiet \
  ./remotr-config
```

Direct database access is an administrative deployment path. In routine
operations, create enrollment/deployment tokens through the mTLS Admin API.

## Compose modules from a manifest

`modules/base-packages.yaml`:

```yaml
kind: module
schemaVersion: 1
configurations:
  - name: base-packages
    description: Portable package baseline
    targetDistros: [Debian, Ubuntu, Arch]
    resources:
      - kind: package
        name: curl
        lifecycle: present
      - kind: package
        name: git
        lifecycle: present
```

`fleets/workstations/manifest.yaml`:

```yaml
kind: manifest
modules:
  - modules/base-packages.yaml
  - modules/access.yaml
applications:
  - chat
crons:
  - crons/weekly-maintenance.yaml
```

Paths are repository-relative. A list entry may also name a directory; the
composer recursively collects files of the expected kind in deterministic
order. Explicit paths are easier to review for high-risk modules.

Configuration names must be unique across all selected modules. Resource names
must be unique across all kinds inside one configuration. The stable address
`<configuration>/<resource-name>` is used by `dependsOn`, reports, change
plans, and baselines.

## Understand composition

For a fleet manifest the composer:

1. resolves `extends` parents depth-first, with cycle and depth protection;
2. resolves selected modules, applications, and cron sources;
3. appends module configuration slices in manifest order;
4. merges selected application packages into the desired artifact;
5. applies manifest overrides by configuration name;
6. validates the complete desired and cron artifacts;
7. stores compiled bytes for the candidate release ref.

If any step fails, Git sync does not advance the release ref. Agents continue
receiving the prior successfully compiled release.

Preview the same result locally:

```bash
remotr config discover --fleet workstations .
remotr config render --fleet workstations .
remotr config validate .
```

`render` writes to stdout unless `--output` is supplied. Use an output path
outside the repository when a review artifact is needed.

## Target multiple distributions

Target configuration slices, not duplicated fleet trees:

```yaml
kind: module
schemaVersion: 1
configurations:
  - name: common
    targetDistros: [Debian, Ubuntu, Arch]
    resources:
      - kind: package
        name: curl
        lifecycle: present

  - name: debian-audit
    targetDistros: [Debian, Ubuntu]
    targetArch: [x86, ARM]
    resources:
      - kind: package
        name: auditd
        lifecycle: present

  - name: arch-audit
    targetDistros: [Arch]
    resources:
      - kind: package
        name: audit
        lifecycle: present
```

The server sends the complete artifact. The agent filters slices using
normalized `/etc/os-release`, architecture, init, package-manager, security,
and network facts. A schema-valid resource can still report `unsupported` on
an endpoint lacking the required provider capability; unsupported state is
never applied as ordinary drift.

## Use dependencies for ordering

```yaml
- kind: file
  name: telemetry-config
  path: /etc/telemetry/config.yaml
  content: |
    enabled: true

- kind: service
  name: telemetry-running
  dependsOn:
    - telemetry/telemetry-config
  provider: systemd
  scope: system
  service: telemetry.service
  enabled: true
  active: true
```

Dependencies use complete addresses even inside the same configuration.
Missing dependencies and cycles fail validation. Without explicit edges,
Remotr uses deterministic kind ordering; do not rely on incidental YAML order
for correctness.

## Add applications

Application files provide a shared catalog for package-shaped state:

```yaml
kind: application
name: company-chat
present: true
packageManager: pwa
pwaURL: https://chat.example.internal
pwaTitle: Company Chat
```

Reference it by explicit path, path below `applications/`, or a unique
basename. If two files share the same basename, use an explicit path.

Application documents intentionally retain their compact package shape. Do
not replace it with a schema-1 `resources` list. See [Applications
format](../reference/applications-format.md).

## Choose the right scheduling model

Server-dispatched job:

```yaml
kind: crons
crons:
  - name: weekly-upgrade
    schedule: "0 2 * * 0"
    timezone: UTC
    commands:
      - name: update
        apply: [apt-get, update]
```

The server marks this job due and returns it at an agent check-in. One missed
slot is dispatched after an offline endpoint returns; Remotr avoids a catch-up
storm.

For work that must run from the OS scheduler while the endpoint is offline,
use a schema-1 `endpointSchedule` resource instead. See [Crons
format](../reference/crons-format.md#server-scheduling-behavior) for the full
comparison.

## Add an endpoint override

The server chooses an endpoint artifact before the fleet artifact. It does not
merge the two at sync time. Compose the complete endpoint result in Git:

`endpoints/<endpoint-id>/manifest.yaml`:

```yaml
kind: manifest
extends: fleets/workstations/manifest.yaml
modules:
  - modules/design-tools.yaml
```

The parent selections are resolved first, then the endpoint's lists are
appended. If you omit `extends`, the endpoint artifact contains only the
endpoint manifest's selections. That is appropriate for a deliberately
isolated machine but dangerous for a small partial override.

Preview it before merge:

```bash
remotr config render --endpoint <endpoint-id> .
```

The endpoint ID comes from enrollment and is shown by `remotr endpoint list`.

## Use manifest overrides carefully

Overrides patch an existing configuration by name and cannot introduce a new
configuration. Their resource fields currently use the legacy grouped
collection shape because the manifest override model is a `Configuration`,
not a canonical `resources` list:

```yaml
kind: manifest
modules:
  - modules/base-packages.yaml
overrides:
  - name: base-packages
    packages:
      - name: curl
        present: true
        packageManager: apt
```

An override's non-empty resource collection replaces that entire collection
for the named configuration; it does not merge individual resources. Prefer a
small additional module when replacement semantics would be surprising.

## Release workflow

Use a branch/PR workflow for every change:

1. create a branch from the tracked release branch;
2. edit modules, applications, crons, or manifests;
3. run `remotr config validate .`;
4. render every affected fleet and endpoint override;
5. inspect targeting, resource addresses, high-risk fields, and secret refs;
6. merge after required review and CI;
7. let webhook/poll Git sync compile the merge commit;
8. confirm the server release ref and endpoint reports.

Manual sync and evidence:

```bash
remotr git sync --json
remotr fleet state report workstations
remotr fleet state report workstations --verbose
remotr logs list --since 1h --json
```

A Git merge is not the same as endpoint convergence. Agents apply on later
syncs, may be offline, may run in `report` mode, or may report a capability or
preflight block.

## High-risk change workflow

Keep access, connectivity, boot, and destructive state in focused modules.
Require:

- a `report`-mode lab/canary fleet first;
- resource-specific preflight and rollback evidence;
- `enforce: true` only after review where the typed resource requires it;
- out-of-band console/recovery access;
- bounded cohorts and explicit verification.

The current generic desired-state path is not automatically gated by the
change-control request/lease system. See [Change control](change-control.md)
for the exact current boundary.

## CI example

The repository includes `.github/workflows/config-repo.yml` as a validation
example. The essential non-mutating gate is:

```bash
remotr config validate .
```

For sensitive repositories, also render each fleet to a protected CI artifact
for review. Rendered output may contain sensitive metadata or local-file paths
even though inline secret bytes are rejected; apply the repository's normal
access controls.

## Troubleshooting composition

| Symptom | Check |
| --- | --- |
| Wrong file selected | Run `config discover`; replace ambiguous application basenames with explicit paths. |
| Duplicate configuration | Ensure every module configuration `name` is unique after `extends`. |
| Duplicate resource | Names are unique across all kinds in a configuration, not merely within one kind. |
| Missing dependency | Use complete `<configuration>/<resource>` addresses. |
| Endpoint lost fleet state | Add `extends: fleets/<fleet>/manifest.yaml` to the endpoint manifest. |
| Release ref did not advance | Inspect Git sync/server logs for composition failure; the previous release remains active. |
| Valid but unsupported | Compare endpoint facts and the provider requirements in the resource reference. |

## Related references

- [Repository file kinds](../reference/repository-kinds.md)
- [Manifest format](../reference/manifest-format.md)
- [Configuration and all resource fields](../reference/configuration-format.md)
- [Resource-kind index](../reference/resource-kinds.md)
- [Configuration validation](config-validation.md)
- [Git sync workflow](git-sync-workflow.md)
