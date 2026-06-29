# Configuration repository

The **configuration repository** is the GitOps source of truth for desired state. Admins review and merge changes in Git; the Remotr server reads a checkout, **composes** deployable artifacts when the release ref advances, and serves artifact bytes to agents at the current **release ref**.

Agents never clone Git directly.

## Repository layout

```text
remotr-config/
├── remotr.yaml                 # operator metadata (not served to agents)
├── server.env.example          # suggested server env vars
├── modules/                    # kind: module — reusable configuration slices
│   └── base-packages.yaml
├── applications/               # kind: application — shared app catalog
│   └── pwa/microsoft/teams.yaml
├── crons/                      # kind: crons — cron job sources
│   └── modules/weekly-upgrade.yaml
├── fleets/
│   └── engineering/
│       └── manifest.yaml       # kind: manifest — fleet entry point
└── endpoints/
    └── <endpoint-id>/
        └── manifest.yaml       # optional kind: manifest — replaces fleet artifact
```

Each YAML file has a required `kind:` field (`manifest`, `module`, `application`, `crons`). The fleet manifest lists modules, applications, and crons by path or folder — there are no separate `applications.manifest.yaml` or `crons.manifest.yaml` files.

Scaffold a new repository:

```bash
remotr init -fleet engineering ./remotr-config
```

![remotr init](../assets/demo/init.gif)

### Modular composition

Split desired state into reusable **modules** under `modules/`, then list them from a fleet **manifest**:

```yaml
kind: manifest
modules:
  - modules/base-packages.yaml
  - modules/sshd-hardening.yaml
applications:
  - slack
crons:
  - crons/modules/weekly-upgrade.yaml
```

Endpoint manifests can extend a fleet and add deltas:

```yaml
kind: manifest
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

Preview composed output locally (does not write files):

```bash
remotr config render .
remotr config render --fleet engineering
remotr config discover --fleet engineering
```

Import reusable modules from the Remotr Hub (run without an entry id in a terminal to pick from the catalog):

```bash
remotr hub snippet import
remotr hub snippet import base-packages-debian-arch
```

See [Manifest format reference](../reference/manifest-format.md) for merge semantics and field reference.

### Fleet artifacts

The server composes desired state and optional crons for each fleet from `fleets/<fleet-name>/manifest.yaml` when Git sync advances the release ref. Composed bytes are cached in Postgres (`compiled_artifacts`).

Every endpoint enrolled in `<fleet-name>` receives this artifact unless an endpoint override manifest exists. The fleet name must match the fleet bound at enrollment time.

### Endpoint overrides

Path: `endpoints/<endpoint-id>/manifest.yaml` (`kind: manifest`)

When present, the composed override **replaces** the fleet artifact for that endpoint only. There is no server-side merge of fleet + override layers — compose divergent state in Git (`extends`, modules, overrides).

The endpoint ID is assigned at enrollment and stored in the agent's `/var/lib/remotr/state.json`.

### Fleet crons

List `kind: crons` files from the fleet manifest `crons:` field. The server composes a crons artifact alongside desired state; cron schedules are evaluated at sync time.

Same override semantics as desired state: an endpoint manifest with its own `crons:` list **replaces** the fleet crons artifact when present (no merge).

Example cron source file:

```yaml
kind: crons
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
remotr config validate .
remotr config validate --json
remotr config validate . --skip-render-check   # kinds and schema only
```

![remotr config validate](../assets/demo/config-validate.gif)

Catches invalid kinds, unresolved references, merge errors, invalid targeting, duplicate resource names, and cron schedule errors before agents see the artifact.

## Release ref and Git sync

The **release ref** is the Git commit SHA the server currently serves. When it advances, agents whose cached digest differs download the new artifact on the next sync.

On each successful fetch/checkout to a new ref, the server runs composition and stores artifacts in Postgres. If composition fails, the release ref **does not** advance.

Advance release ref by:

1. **Webhook** — POST to `/v1/webhooks/git` after push (with `X-Remotr-Git-Webhook-Secret` when configured)
2. **Poll** — `REMOTR_GIT_SYNC_POLL_INTERVAL` triggers periodic `git fetch` + `rev-parse HEAD`
3. **External process** — CI or ops updates the checkout; webhook or poll picks up HEAD

For non-Git mounts (NFS, ConfigMap volume without `.git`), set a static `REMOTR_RELEASE_REF` label. Without Postgres, the server composes on demand at sync time.

## Workflow: change desired state

1. Branch from `main` in the configuration repository.
2. Edit modules and/or `fleets/<fleet>/manifest.yaml` (or an endpoint manifest).
3. Run `remotr config validate .` locally.
4. Open a pull request; reviewers validate YAML and targeting.
5. Merge to the tracked branch.
6. Git sync fetches the merge commit, composes artifacts, and advances release ref on success.
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

1. Create `fleets/<new-fleet>/manifest.yaml` (`kind: manifest`) and shared modules under `modules/` as needed.
2. Run `remotr config validate .` and `remotr config render --fleet <new-fleet>` to preview.
3. Register the fleet in Postgres (`fleet_settings`) with remediation policy.
4. Create enrollment tokens for the new fleet via `remotr enroll token create`.
5. Update `remotr.yaml` metadata if you use it for documentation.

## Validation tips

- Configuration `name` values must be unique within a composed artifact.
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
