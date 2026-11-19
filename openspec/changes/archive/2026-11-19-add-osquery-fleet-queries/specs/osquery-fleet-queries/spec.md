## ADDED Requirements

### Requirement: Fleet osquery queries are GitOps-defined and validated
Operators SHALL define fleet-scoped read-only osquery queries in `kind: queries` (`schemaVersion: 1`) YAML documents referenced from fleet manifests. Each query SHALL declare a unique `name` per fleet, read-only `sql`, `schedule`, optional `timezone`, `mode` (`snapshot` or `diff`, defaulting to `snapshot`), bounded `timeout`, `maxRows`, `retentionRuns`, and optional distro/architecture selectors. Configuration validation and server git-sync composition SHALL reject mutating SQL, multi-statement SQL, `ATTACH`/`PRAGMA`/`LOAD_EXTENSION` directives, invalid schedules, out-of-bounds limits, and duplicate query names within a fleet before advancing the release ref.

#### Scenario: Valid fleet queries compose into a fleet artifact
<!-- verification-id: OS-OSQ-001 -->
- **WHEN** a fleet manifest references a `kind: queries` document containing valid read-only `SELECT`/`WITH` snapshot and diff queries
- **THEN** `remotr config validate` succeeds and server composition stores a compiled per-fleet `queries` artifact in `compiled_artifacts`

#### Scenario: Unsafe or mutating SQL is rejected at validation time
<!-- verification-id: OS-OSQ-002 -->
- **WHEN** a `kind: queries` document contains a non-read-only statement (`DROP`, `ATTACH`, `PRAGMA`, or semicolon-chained statements)
- **THEN** `remotr config validate` and server composition reject the document and block release ref advance

### Requirement: Osquery installation and binary resolution are explicitly controlled without clobbering
A `kind: osquery` resource SHALL explicitly control whether, where, and with what visibility `osquery` is installed or reused on an endpoint via `mode` (`disabled`, `existing`, or `managed`). The agent SHALL NOT implicitly discover or execute `osqueryi` from `$PATH` unless authorized by a `kind: osquery` runtime policy.

#### Scenario: Private managed installation isolates osquery outside user space
<!-- verification-id: OS-OSQ-003 -->
- **WHEN** a fleet declares `kind: osquery` with `mode: managed` and `visibility: private` (the default) plus a pinned `version`, `sourceUrl`, and `sha256`
- **THEN** the applicator verifies the SHA-256 checksum, installs the binary under a `root:root` `0700` private directory outside user `$PATH` (default `/usr/lib/remotr/osquery`), writes no files into `/usr/bin`, `/usr/local/bin`, `/etc/osquery`, or systemd directories, and leaves regular local users unable to execute it

#### Scenario: Global managed installation refuses to clobber unmanaged binaries
<!-- verification-id: OS-OSQ-004 -->
- **WHEN** `kind: osquery` declares `mode: managed` and `visibility: global` and an unmanaged file or symlink already exists at the target global symlink path
- **THEN** Check/Apply fails closed without overwriting or removing the existing unmanaged binary

#### Scenario: Existing mode reuses third-party bundled osquery safely without modifying it
<!-- verification-id: OS-OSQ-005 -->
- **WHEN** `kind: osquery` declares `mode: existing` pointing to an existing binary path or anchored pattern (such as Elastic Agent's bundled `osqueryd`)
- **THEN** the agent resolves symlinks, verifies regular-file type, permitted ownership, and non-world/group-writable permissions, invokes `osqueryd` with `-S --json --ephemeral --disable_database --disable_events` (or `osqueryi --json --ephemeral --disable_database --disable_events`), and never modifies or deletes that binary even when `lifecycle: absent`

#### Scenario: Disabled or missing runtime reports unsupported cleanly
<!-- verification-id: OS-OSQ-006 -->
- **WHEN** `mode: disabled` is declared or no configured `osquery` binary is present on an endpoint
- **THEN** the agent advertises `osquery` as unavailable and reports `unsupported` for any dispatched query without attempting to install software or fall back to `$PATH`

### Requirement: Pull-based scheduled execution enforces strict bounds and tracks snapshots and diffs
The server SHALL dispatch due scheduled queries to eligible endpoints during authenticated `/sync` check-ins. The agent SHALL execute each query via exact argv (`--json --ephemeral --disable_database --disable_events`, prefixed with `-S` for `osqueryd`) without shell interpolation, enforcing per-query `timeout`, `maxRows`, and payload byte caps, and return bounded results with explicit truncation indicators. On ingestion, the server SHALL persist execution status, update the latest per-endpoint snapshot, and for `mode: diff` queries compute canonical `added_rows` and `removed_rows` against the previous successful snapshot while pruning history to `retentionRuns`.

#### Scenario: Scheduled snapshot query runs with exact argv and bounds
<!-- verification-id: OS-OSQ-007 -->
- **WHEN** a scheduled `snapshot` query becomes due for an eligible endpoint
- **THEN** the agent executes the configured binary with exact `--json --ephemeral --disable_database --disable_events` arguments, caps rows at `maxRows` (setting `truncated: true` if exceeded), enforces `timeout`, and the server stores the execution record and latest snapshot

#### Scenario: Diff query computes server-side added and removed rows
<!-- verification-id: OS-OSQ-008 -->
- **WHEN** a `mode: diff` query completes on an endpoint whose prior snapshot exists
- **THEN** the server canonicalizes row keys, persists the exact `added_rows` and `removed_rows` in `query_diffs`, updates `query_snapshots`, and prunes older diff entries beyond `retentionRuns`

### Requirement: Operators can trigger RBAC-gated ad-hoc live queries
The server and CLI SHALL allow authorized operators to trigger on-demand live queries (`remotr query run`) targeting a single endpoint or an entire fleet, referencing either a fleet-declared query name (`query:run`) or an inline validated read-only SQL statement (`query:run_sql`). Ad-hoc requests SHALL carry a bounded expiry TTL, be recorded in the audit log, be dispatched on endpoint check-in, and expire cleanly if endpoints are offline.

#### Scenario: Authorized operator runs an ad-hoc named query and receives results
<!-- verification-id: OS-OSQ-009 -->
- **WHEN** an operator with `query:run` invokes `remotr query run <name> --endpoint <id>`
- **THEN** the server creates a TTL-bounded ad-hoc request, audits the action, dispatches it on the endpoint's next check-in, and the CLI streams/displays the returned rows upon completion

#### Scenario: Inline ad-hoc SQL requires dedicated permission and read-only validation
<!-- verification-id: OS-OSQ-010 -->
- **WHEN** an operator without `query:run_sql` submits `--sql`, or submits a non-read-only inline SQL statement
- **THEN** the server rejects the request with HTTP 403 (unauthorized) or HTTP 400 (invalid SQL) without dispatching anything to endpoints

### Requirement: Operator CLI provides ergonomic fleet query inspection and export
The `remotr query` command SHALL provide `list`, `status`, `results`, `diff`, and `run` subcommands gated by `query:read` (and `query:run`/`query:run_sql` for `run`). `results` SHALL support flattened fleet tables with a coverage summary header, automatic key-value card fallback for wide results, `--group-by` anomaly aggregation across endpoints, interactive `--tui` browsing (live filtering, sorting, view switching), and `--format table|json|ndjson|csv` with `--save`.

#### Scenario: Fleet results render flattened table and anomaly-grouped aggregation
<!-- verification-id: OS-OSQ-011 -->
- **WHEN** an operator runs `remotr query results <name> --fleet <f>` and with `--group-by`
- **THEN** default output prints a coverage header plus flattened `ENDPOINT`-prefixed rows (or cards on narrow terminals), and `--group-by` aggregates identical rows into `COUNT`, sample endpoints, and column values sorted to highlight rare outliers

#### Scenario: Diff view and machine export preserve fidelity and truncation warnings
<!-- verification-id: OS-OSQ-012 -->
- **WHEN** an operator runs `remotr query diff <name>` or `remotr query results <name> --format csv --save`
- **THEN** `diff` displays `+`/`-` row changes per endpoint with timestamps, machine formats emit valid JSON/NDJSON/CSV, and any truncated endpoint result is clearly flagged in output metadata
