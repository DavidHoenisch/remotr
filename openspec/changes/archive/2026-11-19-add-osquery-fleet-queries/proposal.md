## Why

Remotr operators currently lack a structured, GitOps-managed way to ask arbitrary read-only SQL questions across Linux fleets, track host state changes over time, or run ad-hoc live security/inventory queries without writing ad-hoc scripts. At the same time, production endpoints often run security agents (such as Elastic Agent) that ship their own local `osquery` binary, and security teams need explicit control over whether `osquery` is installed at all, whether an existing binary is reused, or whether Remotr installs a private copy outside regular users' execution scope so nothing is clobbered or exposed in user space.

## What Changes

- **GitOps fleet query authoring (`kind: queries`, `schemaVersion: 1`)**:
  - Introduce `kind: queries` YAML documents referenced by fleet manifests (matching `kind: crons`), declaring query `name`, `description`, read-only `sql`, `schedule`, `timezone`, `mode` (`snapshot` or `diff`), `timeout`, `maxRows`, `retentionRuns`, and OS/arch selectors (`targetDistros`, `targetArch`).
  - Validate read-only SQL (`SELECT`/`WITH` only, single statement, no `ATTACH`/pragmas), schedules, bounds, and per-fleet name uniqueness in `remotr config validate` and server composition into `compiled_artifacts`.
- **Flexible, non-clobbering `osquery` installation and binary resolution**:
  - Define an explicit `osquery` runtime/install policy (declarable per fleet/configuration) controlling **if**, **when**, and **where** `osqueryi` is installed or resolved:
    - `mode: disabled`: never install; endpoints without a configured runtime report `unsupported` for query runs.
    - `mode: existing`: reuse an existing `osqueryi` binary (e.g., Elastic Agent's bundled `osqueryi` or a pre-installed system binary) at an operator-specified absolute path or ordered allowlist of paths, verified for ownership/permissions/read-only capability without installing or modifying external tools.
    - `mode: managed`: Remotr installs a checksum-verified `osquery` binary with explicit isolation scope:
      - `visibility: private` (default): installs into a root-owned private directory outside user `$PATH` (default `/usr/lib/remotr/osquery`, mode `0700` directory, `root:root` ownership) so regular local users cannot discover or execute it and other agents (such as Elastic Agent) are never clobbered.
      - `visibility: global`: opt-in system-wide installation (e.g., `/usr/local/bin/osqueryi` or managed system path) accessible to regular users when desired.
- **Pull-based scheduled snapshots, server-side diffs, and ad-hoc live queries**:
  - Dispatch due scheduled queries (`snapshot` and `diff` modes) and TTL-bounded ad-hoc live query requests over the existing authenticated agent `/sync` loop.
  - Execute queries on the agent via exact-argv `osqueryi --json --ephemeral --disable_database --disable_events` (never via a shell) with strict timeout, row count, and payload byte limits.
  - Persist executions, latest per-endpoint snapshots, server-computed row diffs (`added_rows`/`removed_rows`), and TTL-expiring ad-hoc runs in Postgres with RBAC permissions (`query:read`, `query:run`, `query:run_sql`).
- **Ergonomic operator CLI (`remotr query`)**:
  - Add top-level `remotr query` subcommands: `list`, `status`, `results`, `diff`, and `run`.
  - Support flattened fleet tables (auto-switching to key-value cards on wide results), `--group-by` anomaly/frequency aggregation across large fleets, colored `+`/`-` diff output, an interactive Bubble Tea viewer (`--tui`), and machine export (`--format table|json|ndjson|csv`, `--save`).

## Capabilities

### New Capabilities

- `osquery-fleet-queries`: GitOps-scoped fleet osquery definitions, flexible private/global/reuse runtime installation policy, pull-based snapshot/diff/ad-hoc execution and storage, RBAC controls, and `remotr query` CLI/TUI result ergonomics.

### Modified Capabilities

None.

## Impact

- `internal/types`, `internal/models`, `internal/configrepo`, `internal/configcompose` (`kind: queries` + `kind: osquery` runtime resource/policy, SQL/schedule/path validation, fleet composition).
- `internal/applicators/osquery` and `internal/resourceregistry` (private/global/existing runtime convergence, checksum verification, non-clobbering install/removal, capability advertisement).
- `internal/queryscheduler`, `internal/agent/queryexec`, `internal/agent/sync` (due-query & ad-hoc dispatch, bounded `osqueryi --json --ephemeral --disable_database --disable_events` execution, result upload).
- `internal/store/postgres` and SQL migrations (`query_executions`, `query_snapshots`, `query_diffs`, `query_adhoc_requests`, `query_adhoc_results`), `internal/rbac`, `internal/server`, `internal/admin`.
- `cmd/remotr` (`remotr query list|status|results|diff|run` + Bubble Tea TUI viewer) and `docs/`.

## Non-goals

- Managing `osqueryd` daemons, event-driven pub/sub tables (`process_events`, `file_events`), or local disk log shipping in this change.
- Allowing mutating SQL statements, SQLite `ATTACH`, or arbitrary shell execution.
- Modifying, upgrading, or uninstalling third-party bundled `osquery` installations (such as Elastic Agent's install) when `mode: existing` is used.
