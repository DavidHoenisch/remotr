# Configuration repository

The **configuration repository** is the GitOps source of truth for desired state. Admins review and merge changes in Git; the Remotr server reads a checkout and serves artifact bytes to agents at the current **release ref**.

Agents never clone Git directly.

## Repository layout

```text
remotr-config/
├── remotr.yaml                 # operator metadata (not served to agents)
├── server.env.example          # suggested server env vars
├── fleets/
│   └── engineering/
│       └── desired.yaml        # deployable artifact for the fleet
└── endpoints/
    └── <endpoint-id>/
        └── desired.yaml        # optional override (replaces fleet file)
```

Scaffold a new repository:

```bash
remotr init -fleet engineering ./remotr-config
```

### Fleet artifacts

Path: `fleets/<fleet-name>/desired.yaml`

Every endpoint enrolled in `<fleet-name>` receives this file unless an endpoint override exists. The fleet name must match the fleet bound at enrollment time.

### Endpoint overrides

Path: `endpoints/<endpoint-id>/desired.yaml`

When present, this file **replaces** the fleet artifact for that endpoint only. There is no server-side merge of fleet + override layers — compose divergent state in Git (separate files, CI rendering, or copy-and-edit workflows).

The endpoint ID is assigned at enrollment and stored in the agent's `/var/lib/remotr/state.json`.

### remotr.yaml

Operator-facing metadata: default fleet, remediation policy hints, path conventions. The server does not read this file when serving agents.

### Validate before push

Run locally from the repository root or a fleet directory:

```bash
remotr config validate .
remotr config validate --json
```

Catches structural issues, invalid targeting, and duplicate resource names before agents see the artifact.

## Release ref and Git sync

The **release ref** is the Git commit SHA the server currently serves. When it advances, agents whose cached digest differs download the new artifact on the next sync.

Advance release ref by:

1. **Webhook** — POST to `/v1/webhooks/git` after push (with `X-Remotr-Git-Webhook-Secret` when configured)
2. **Poll** — `REMOTR_GIT_SYNC_POLL_INTERVAL` triggers periodic `git fetch` + `rev-parse HEAD`
3. **External process** — CI or ops updates the checkout; webhook or poll picks up HEAD

For non-Git mounts (NFS, ConfigMap volume without `.git`), set a static `REMOTR_RELEASE_REF` label. Agents still receive digest changes when file content changes, but ref advancement is manual.

## Workflow: change desired state

1. Branch from `main` in the configuration repository.
2. Edit `fleets/<fleet>/desired.yaml` (or an endpoint override).
3. Open a pull request; reviewers validate YAML and targeting.
4. Merge to the tracked branch.
5. Git sync advances release ref on the server.
6. Agents sync within their poll interval (`REMOTR_SYNC_INTERVAL`, default 30s).

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

1. Create `fleets/<new-fleet>/desired.yaml`.
2. Register the fleet in Postgres (`fleet_settings`) with remediation policy.
3. Create enrollment tokens for the new fleet via `remotr enroll token create`.
4. Update `remotr.yaml` metadata if you use it for documentation.

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

- [Configuration format reference](../reference/configuration-format.md)
- [Operator workflows](operator-workflows.md)
- [Architecture — resolved desired state](../explanation/architecture.md#from-artifact-to-apply)
