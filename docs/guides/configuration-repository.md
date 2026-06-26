# Configuration repository

The **configuration repository** is the GitOps source of truth for desired state. Admins review and merge changes in Git; the Remotr server reads a checkout and serves artifact bytes to agents at the current **release ref**.

Agents never clone Git directly.

## Repository layout

```text
remotr-config/
├── remotr.yaml                 # operator metadata (not served to agents)
├── server.env.example          # suggested server env vars
├── modules/                    # reusable configuration slices (source)
│   ├── base-packages.yaml
│   └── ssh-hardening.yaml
├── crons/
│   └── builtin/                # optional shared cron templates
├── fleets/
│   └── engineering/
│       ├── manifest.yaml       # composition source (lists modules)
│       ├── desired.yaml        # generated deployable artifact
│       └── crons.yaml          # optional scheduled jobs for the fleet
└── endpoints/
    └── <endpoint-id>/
        ├── manifest.yaml       # optional composition (extends fleet)
        ├── desired.yaml        # generated override (replaces fleet file)
        └── crons.yaml          # optional override (replaces fleet crons)
```

Scaffold a new repository:

```bash
remotr init -fleet engineering ./remotr-config
```

![remotr init](../assets/demo/init.gif)

### Modular composition

Split desired state into reusable **modules** under `modules/`, then list them from a fleet or endpoint **manifest**:

```yaml
# fleets/engineering/manifest.yaml
modules:
  - modules/base-packages.yaml
  - modules/sshd-hardening.yaml
```

Endpoint manifests can extend a fleet and add deltas without copying the full fleet file:

```yaml
# endpoints/workstation-42/manifest.yaml
extends: fleets/engineering/manifest.yaml
modules:
  - modules/designer-extra.yaml
overrides:
  - name: base-packages
    packages:
      - name: vim
        present: true
        packageManager: apt
```

Generate flat artifacts before push:

```bash
remotr config compose .
remotr config compose . --check    # CI: fail when artifacts are stale
remotr config compose . --dry-run  # show diffs when artifacts would change; no output when up to date
remotr config compose . --fleet engineering --print          # print composed desired.yaml
remotr config compose . --fleet engineering --stdout crons   # print composed crons.yaml
```

Import reusable modules from the Remotr Hub (run without an entry id in a terminal to pick from the catalog):

```bash
remotr hub snippet import
remotr hub snippet import base-packages-debian-arch
```

See [Manifest format reference](../reference/manifest-format.md) for merge semantics and crons manifests.

### Fleet artifacts

Path: `fleets/<fleet-name>/desired.yaml` (generated from `manifest.yaml` when using modular layout)

Every endpoint enrolled in `<fleet-name>` receives this file unless an endpoint override exists. The fleet name must match the fleet bound at enrollment time.

### Endpoint overrides

Path: `endpoints/<endpoint-id>/desired.yaml`

When present, this file **replaces** the fleet artifact for that endpoint only. There is no server-side merge of fleet + override layers — compose divergent state in Git (separate files, CI rendering, or copy-and-edit workflows).

The endpoint ID is assigned at enrollment and stored in the agent's `/var/lib/remotr/state.json`.

### Fleet crons

Path: `fleets/<fleet-name>/crons.yaml`

Optional file defining **scheduled jobs** for endpoints in the fleet. The server evaluates cron schedules and dispatches work on agent sync; execution history is stored in Postgres.

Same override semantics as desired state: `endpoints/<endpoint-id>/crons.yaml` **replaces** the fleet file when present (no merge).

Example using a builtin template:

```yaml
crons:
  - use: builtin/system-upgrade
    schedule: "0 0 * * 0"
```

See [Crons format reference](../reference/crons-format.md) for schedule syntax, builtins, and custom jobs.

### remotr.yaml

Operator-facing metadata: default fleet, remediation policy hints, path conventions. The server does not read this file when serving agents.

### Validate before push

Run locally from the repository root:

```bash
remotr config compose . --check
remotr config validate .
remotr config validate --json
```

![remotr config validate](../assets/demo/config-validate.gif)

Catches structural issues, invalid targeting, duplicate resource names, and cron schedule errors before agents see the artifact.

## Release ref and Git sync

The **release ref** is the Git commit SHA the server currently serves. When it advances, agents whose cached digest differs download the new artifact on the next sync.

Advance release ref by:

1. **Webhook** — POST to `/v1/webhooks/git` after push (with `X-Remotr-Git-Webhook-Secret` when configured)
2. **Poll** — `REMOTR_GIT_SYNC_POLL_INTERVAL` triggers periodic `git fetch` + `rev-parse HEAD`
3. **External process** — CI or ops updates the checkout; webhook or poll picks up HEAD

For non-Git mounts (NFS, ConfigMap volume without `.git`), set a static `REMOTR_RELEASE_REF` label. Agents still receive digest changes when file content changes, but ref advancement is manual.

## Workflow: change desired state

1. Branch from `main` in the configuration repository.
2. Edit modules and/or `fleets/<fleet>/manifest.yaml` (or an endpoint manifest).
3. Run `remotr config compose .` and commit generated `desired.yaml` when using modular layout.
4. Open a pull request; reviewers validate YAML and targeting.
5. Merge to the tracked branch.
6. Git sync advances release ref on the server.
7. Agents sync within their poll interval (`REMOTR_SYNC_INTERVAL`, default 30s).

Use `report` remediation policy on lab fleets to observe drift without automatic apply. See [Configuration format](../reference/configuration-format.md) for resource kinds.

## Multi-distro fleets

One fleet artifact can target multiple distros using **in-document targeting** on each configuration slice:

```yaml
configurations:
  - name: base-packages
    targetDistros:
      - Debian
      - Arch
    packages:
      - name: curl
        present: true
        packageManager: apt
      - name: curl
        present: true
        packageManager: pacman
```

The server sends the full file. Each agent filters stanzas locally using `/etc/os-release` and `uname -m`.

## Adding a fleet

1. Create `fleets/<new-fleet>/manifest.yaml` (and shared modules under `modules/` as needed).
2. Run `remotr config compose .` to generate `desired.yaml`.
3. Register the fleet in Postgres (`fleet_settings`) with remediation policy.
4. Create enrollment tokens for the new fleet via `remotr enroll token create`.
5. Update `remotr.yaml` metadata if you use it for documentation.

## Validation tips

- Configuration `name` values must be unique within a file.
- Resource `name` values must be unique within a configuration slice.
- `dependsOn` references use `configuration-name/resource-name`.
- Duplicate resource addresses are parse errors.
- Dependency cycles fail at agent pre-apply time.

Run unit tests after editing parsers or adding examples:

```bash
go test -mod=vendor ./internal/models/...
```

## Related docs

- [Manifest format reference](../reference/manifest-format.md)
- [Configuration format reference](../reference/configuration-format.md)
- [Crons format reference](../reference/crons-format.md)
- [Operator overview](operator-overview.md)
- [Architecture — resolved desired state](../explanation/architecture.md#from-artifact-to-apply)
