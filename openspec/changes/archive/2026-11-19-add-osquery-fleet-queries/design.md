## Context

Remotr currently delivers desired-state configuration (`kind: module`) and scheduled maintenance jobs (`kind: crons`) over an authenticated pull-based `/sync` protocol, storing per-fleet compiled artifacts in Postgres (`compiled_artifacts`) and presenting fleet/endpoint state via `remotr` CLI subcommands (`inventory`, `endpoint`, `fleet`, `diagnostics`, `logs`, `config`) including a Bubble Tea interactive viewer (`cmd/remotr/diagnostics_viewer.go`).

Operators want to author read-only osquery SQL queries in GitOps YAML, scope them to fleets, run them on schedules (both snapshot and diff modes) as well as on demand, and review results ergonomically from the CLI. At the same time, endpoints commonly run third-party security tooling (such as Elastic Agent) that bundles its own local `osquery` binary, and operators need strict control over **if**, **when**, and **where** `osquery` is installed — including installing into a root-private path outside user `$PATH` or safely reusing an existing binary without clobbering external tools.

## Goals / Non-Goals

**Goals:**

- Introduce `kind: queries` (`schemaVersion: 1`) YAML documents composed per fleet into `compiled_artifacts` alongside `desired` and `crons`.
- Introduce a first-class `kind: osquery` runtime/installation resource supporting three explicit modes (`disabled`, `existing`, `managed`) and two visibility scopes for managed installs (`private` default vs. `global` opt-in), guaranteeing zero clobbering of third-party agents (such as Elastic Agent) and zero unintended user-space exposure.
- Support all three query execution workflows over the existing authenticated `/sync` channel:
  1. Scheduled **snapshots** (`mode: snapshot`) storing latest per-endpoint rows.
  2. Scheduled **diffs** (`mode: diff`) with server-side row-set diffing (`added_rows` / `removed_rows`) so the agent remains stateless.
  3. **Ad-hoc live queries** (`remotr query run`) for named GitOps queries or RBAC-gated inline SQL with TTL expiry.
- Provide ergonomic `remotr query` CLI views: flattened fleet tables (with wide-table card fallback), `--group-by` anomaly aggregation, colored `+`/`-` diff output, interactive Bubble Tea `--tui` browsing, and `json`/`ndjson`/`csv` export with `--save`.
- Enforce strict execution, payload, SQL read-only, binary-trust, and RBAC guardrails.

**Non-Goals:**

- Managing `osqueryd` long-running daemons, pub/sub event tables (`process_events`, `file_events`), or local disk log shipping.
- Modifying, upgrading, or removing externally owned `osquery` installations (e.g., Elastic Agent's bundled binary) under `mode: existing` or `lifecycle: absent`.
- Executing arbitrary mutating SQL, multi-statement SQL, SQLite `ATTACH`, or shell pipelines.

## Decisions

### 1. Explicit `kind: osquery` runtime policy (`disabled` | `existing` | `managed`)

- **Decision:** Separate **runtime provisioning/resolution** (`kind: osquery` inside a fleet module) from **query authoring** (`kind: queries`). Never auto-discover `osqueryi` implicitly from `$PATH` without an explicit runtime declaration.
  - **`mode: disabled`**: Do not install or resolve `osquery`. Any scheduled or ad-hoc query dispatched to the endpoint reports `unsupported`.
  - **`mode: existing`**: Reuse an externally managed binary declared via `binaryPaths[]` (exact absolute paths or single-segment anchored path patterns such as `/opt/Elastic/Agent/data/elastic-agent-*/components/osqueryd`).
    - Supports both `osqueryi` (`osqueryi --json --ephemeral --disable_database --disable_events`) and standalone-shell `osqueryd` (`osqueryd -S --json --ephemeral --disable_database --disable_events`), which directly accommodates Elastic Agent's bundled `osqueryd`.
    - Before trusting a candidate binary, the agent resolves symlinks (`filepath.EvalSymlinks`), verifies it is a regular executable file, confirms ownership matches `root` (or explicitly declared `allowedOwner`), and rejects group- or world-writable binaries/ancestors (fail-closed against local privilege escalation).
    - Remotr never writes to, chmods, upgrades, or deletes an `existing` binary — even when `lifecycle: absent`.
  - **`mode: managed`**: Remotr downloads and verifies a pinned `osquery` artifact (`version`, `sourceUrl`, `sha256`) using atomic installation and transactional rollback:
    - **`visibility: private` (default)**: Installs into a root-owned private directory outside normal user `$PATH` (default `/usr/lib/remotr/osquery`, configurable via `installDir`), with `root:root` ownership and `0700` directory + `0700` binary permissions. Regular local users cannot list, discover, or execute the binary; zero files are placed in `/usr/bin`, `/usr/local/bin`, `/etc/osquery`, or systemd unit directories, guaranteeing zero conflict with Elastic Agent or distro packages.
    - **`visibility: global` (opt-in)**: Installs into a versioned Remotr-owned directory (default `/opt/remotr-osquery`, `0755`) and manages a symlink at `/usr/local/bin/osqueryi` (or configured `symlinkPath`). Refuses to overwrite an existing non-Remotr-owned file or symlink at the global path (fail-closed anti-clobbering).
    - `lifecycle: absent` removes only Remotr's owned private directory and managed symlink (verified by an ownership marker), leaving external binaries untouched.
- **Alternative considered:** Installing the upstream distro `osquery` deb/rpm package by default. Rejected because distro packages install globally into `/usr/bin/osqueryi` (exposing it to all local users), register `/etc/osquery` and `osqueryd.service`, and can collide with existing security tooling.

### 2. Pull-based scheduling (`queryscheduler`) + stateless agent + server-side diffs

- **Decision:** Reuse the `cronscheduler` pull-dispatch architecture (`internal/cronscheduler/scheduler.go` + `internal/server/crons.go`) for scheduled queries and the `diagnostics collect` request/poll architecture (`internal/server/diagnostics.go`) for ad-hoc live queries:
  - During `POST /sync`, the server computes `due_queries[]` (from compiled `queries` artifacts + `query_executions` history) and `adhoc_queries[]` (pending non-expired rows in `query_adhoc_requests` targeting the endpoint or its fleet).
  - The agent executes each query sequentially with exact argv (`executil.Runner`, no shell), `--json --ephemeral --disable_database --disable_events` (plus `-S` when using `osqueryd`), per-query `timeout` (default `15s`, max `60s`), `maxRows` (default `500`, max `5000`), and `maxBytes` (default `1 MiB`).
  - The agent posts bounded results back to `POST /sync/query-results`.
  - On ingestion, `remotr-server`:
    1. Records the run in `query_executions`.
    2. For `mode: diff`, canonicalizes each row object (sorted keys, stable JSON encoding) and computes the multiset/set delta (`added_rows`, `removed_rows`) against the endpoint's previous `query_snapshots` entry before updating `query_snapshots`, storing the delta in `query_diffs` and pruning runs beyond `retentionRuns`.
    3. For `mode: snapshot`, replaces `query_snapshots` in place.
- **Alternative considered:** Computing diffs on the endpoint agent. Rejected because keeping diff state on the server avoids local SQLite/JSON state corruption, supports re-indexing/retention centrally in Postgres, and keeps the agent stateless and lightweight.

### 3. Read-only SQL validation at authoring, composition, and execution

- **Decision:** Validate SQL strictly in `internal/models/query.go` (shared by `remotr config validate`, server composition, ad-hoc API intake, and agent pre-exec):
  - Must be a single statement starting with `SELECT` or `WITH` (after stripping comments/leading whitespace).
  - Reject semicolons separating multiple statements, `ATTACH`, `DETACH`, `PRAGMA`, `INSERT`, `UPDATE`, `DELETE`, `DROP`, `ALTER`, `CREATE`, `REPLACE`, `VACUUM`, `LOAD_EXTENSION`.
  - Always pass `--ephemeral --disable_database --disable_events` to `osqueryi`/`osqueryd -S` so ad-hoc and scheduled runs never touch RocksDB state or conflict with Elastic Agent.

### 4. Postgres schema and bounded retention

- **Decision:** Add focused SQL migrations and `sqlc` queries for:
  - `query_executions`: `(run_id PK, endpoint_id, fleet, query_name, trigger_type, status, row_count, truncated, duration_ms, error_summary, scheduled_for, started_at, completed_at)` indexed on `(fleet, query_name, completed_at DESC)` and `(endpoint_id, query_name, completed_at DESC)`.
  - `query_snapshots`: `(endpoint_id, query_name) PK, fleet, run_id, rows JSONB, row_count, truncated, updated_at`.
  - `query_diffs`: `(run_id PK, endpoint_id, fleet, query_name, added_rows JSONB, removed_rows JSONB, added_count, removed_count, created_at)` pruned per `(endpoint_id, query_name)` to `retentionRuns` (default `20`, max `100`).
  - `query_adhoc_requests` and `query_adhoc_results`: tracks operator-initiated live queries with `expires_at` TTL (default `10m`, max `1h`) and automatic retention cleanup.

### 5. CLI placement (`remotr query`) and display ergonomics

- **Decision:** Add a top-level `remotr query` command (`cmd/remotr/cmd_query.go` + `cmd/remotr/query_viewer.go`) with subcommands:
  - `remotr query list [--fleet <f>]`
  - `remotr query status <name> [--fleet <f>] [--only-errors] [--stale]`
  - `remotr query results <name> [--fleet <f> | --endpoint <id>]`
  - `remotr query diff <name> [--fleet <f> | --endpoint <id>] [--since <dur/rfc3339>] [--limit N]`
  - `remotr query run [<name> | --sql "..."] [--fleet <f> | --endpoint <id>] [--timeout 2m]`
- **Ergonomic display modes:**
  - **Default TTY table:** Flattens fleet results by prepending `ENDPOINT`, renders a compact coverage header (`40/42 endpoints · 118 rows · 2 stale · 0 failed`), truncates wide values cleanly, and switches to stacked vertical key-value cards when total column width exceeds terminal width (unless `--wide`).
  - **Anomaly aggregation (`--group-by` / `--count`):** Groups identical result rows across endpoints into `COUNT | SAMPLE ENDPOINTS | <columns...>` sorted rarest-first (default) or `--sort desc` so fleet outliers surface immediately.
  - **Diff output:** Renders grouped `+` / `-` row changes per endpoint with timestamps (colorized in TTY, plain in pipes).
  - **Interactive viewer (`--tui`):** Reuses Bubble Tea (`cmd/remotr/diagnostics_viewer.go` pattern) with `/` live filter, column sort, `g` toggle between raw / grouped / endpoint drill-down, and `e` export.
  - **Machine export:** `--format table|json|ndjson|csv` plus `--save` (matching `remotr inventory --save`), `--where col=val`, `--non-empty`, and `--only-errors`.
- **RBAC permissions:** Add three granular permissions in `internal/rbac`:
  - `query:read` (`list`, `status`, `results`, `diff`)
  - `query:run` (ad-hoc `run` of a fleet-declared GitOps query name)
  - `query:run_sql` (ad-hoc `run --sql` of arbitrary validated read-only SQL)

## Risks / Trade-offs

- **[Risk] Elastic Agent upgrades rotate the hashed directory name under `/opt/Elastic/Agent/data/elastic-agent-<hash>/`** → Mitigation: support anchored single-segment wildcard patterns in `binaryPaths[]` resolved dynamically on each agent check-in/capability refresh, re-verifying ownership and non-world-writable permissions every time before execution.
- **[Risk] Large fleet query result payloads bloat Postgres or saturate `/sync`** → Mitigation: enforce per-query `maxRows` (default 500, cap 5000), per-run `maxBytes` (default 1 MiB), server-side `retentionRuns` pruning, TTL cleanup for ad-hoc tables, and bounded concurrent check-in dispatch (jittered via existing polling).
- **[Risk] Sensitive host data exposed via query results or inline ad-hoc SQL** → Mitigation: separate `query:read`, `query:run`, and `query:run_sql` RBAC permissions, audit-log every ad-hoc query request (`actor`, `target`, `sql`, `timestamp`), and keep `osqueryi` read-only with no shell interpolation.
- **[Risk] Managed `osquery` download integrity or partial install failure** → Mitigation: mandatory SHA-256 verification before staging, atomic rename activation, transactional rollback via `rollbackstore`, and strict refusal to overwrite unmanaged paths in `visibility: global` mode.

## Migration Plan

- Purely additive across config repo (`kind: queries`, `kind: osquery`), Postgres migrations, `/sync` JSON fields (ignored by older agents), RBAC defaults, and CLI commands.
- Existing fleets without a `kind: osquery` resource or `kind: queries` files experience zero behavior change and zero installation activity.
- Fleets can roll out `kind: osquery` (`mode: existing` or `mode: managed` with `visibility: private`) first, verify capability advertisement via `remotr endpoint show` / `remotr query status`, and then attach `kind: queries` files incrementally.
