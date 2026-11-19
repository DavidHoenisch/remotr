## 1. GitOps Query & Osquery Runtime Models and Composition

- [x] 1.1 Add `types.KindQueries` (`kind: queries`) and `models.QuerySpec` / `models.QueryState` with read-only SQL validation (`SELECT`/`WITH`, single statement, rejecting `ATTACH`/`PRAGMA`/DDL/DML), schedule/bounds validation, unit table tests, and bounded fuzzing (`OS-OSQ-001`, `OS-OSQ-002`).
- [x] 1.2 Wire `kind: queries` discovery, fleet manifest references, `remotr config validate` / `config render`, and server git-sync compilation into `compiled_artifacts` (`OS-OSQ-001`, `OS-OSQ-002`).
- [x] 1.3 Add `models.OsqueryResource` (`kind: osquery`, `mode: disabled|existing|managed`, `visibility: private|global`, `installDir`, `symlinkPath`, `binaryPaths`, `allowedOwner`, `version`, `sourceUrl`, `sha256`), register it in `internal/resourceregistry` with explicit field redaction policies, and add unit + fuzz validation tests (`OS-OSQ-003`, `OS-OSQ-004`, `OS-OSQ-005`, `OS-OSQ-006`).

## 2. Osquery Runtime Applicator & Agent Execution Engine

- [x] 2.1 Implement `internal/applicators/osquery` supporting `mode: managed` (`visibility: private` `0700` root-owned directory outside `$PATH` vs. `visibility: global` anti-clobber symlink check), SHA-256 verification, atomic install, transactional rollback, and ownership-marker-gated removal (`OS-OSQ-003`, `OS-OSQ-004`).
- [x] 2.2 Implement `mode: existing` binary resolution (exact paths and anchored single-segment patterns such as Elastic Agent's `osqueryd`, symlink resolution, owner & non-group/world-writable verification, `osqueryi` vs. `osqueryd -S` detection, non-destructive `absent`) and `mode: disabled` runtime state persistence + capability advertisement (`OS-OSQ-005`, `OS-OSQ-006`).
- [x] 2.3 Implement `internal/agent/queryexec` executing queries with exact argv (`--json --ephemeral --disable_database --disable_events`, plus `-S` for `osqueryd`), context timeouts, `maxRows` truncation flagging, `maxBytes` caps, sanitized error summaries, and `unsupported` reporting when runtime is unavailable (`OS-OSQ-006`, `OS-OSQ-007`).

## 3. Server Scheduling, Diff Engine, Persistence, and Ad-Hoc API

- [x] 3.1 Add Postgres migrations and `sqlc` queries for `query_executions`, `query_snapshots`, `query_diffs`, `query_adhoc_requests`, and `query_adhoc_results` with retention pruning and TTL expiry cleanup (`OS-OSQ-007`, `OS-OSQ-008`, `OS-OSQ-009`).
- [x] 3.2 Implement deterministic `internal/queryscheduler` (injected clock, due-slot calculation, stale-run recovery) and server-side canonical row-set diff computation (`added_rows`, `removed_rows`) on result ingestion (`OS-OSQ-007`, `OS-OSQ-008`).
- [x] 3.3 Extend authenticated `/sync` and `POST /sync/query-results` endpoints to dispatch due scheduled queries and pending ad-hoc requests and ingest bounded results (`OS-OSQ-007`, `OS-OSQ-008`, `OS-OSQ-009`).
- [x] 3.4 Add RBAC permissions (`query:read`, `query:run`, `query:run_sql`), audit logging for ad-hoc requests, and Admin API handlers (`/admin/queries`, `/admin/queries/{name}/status`, `/admin/queries/{name}/results`, `/admin/queries/{name}/diff`, `/admin/queries/run`, `/admin/queries/runs/{id}`) with auth/RBAC/negative integration tests (`OS-OSQ-009`, `OS-OSQ-010`).

## 4. Operator CLI (`remotr query`) and Interactive TUI Viewer

- [x] 4.1 Implement `internal/admin` client methods and `cmd/remotr/cmd_query.go` subcommands (`list`, `status`, `results`, `diff`, `run`) with coverage headers, flattened `ENDPOINT`-prefixed tables, automatic wide-table card fallback, `--where`, `--non-empty`, `--only-errors`, `--format table|json|ndjson|csv`, and `--save` (`OS-OSQ-009`, `OS-OSQ-011`, `OS-OSQ-012`).
- [x] 4.2 Implement `--group-by` / `--count` fleet anomaly aggregation and colored `+`/`-` diff formatting with unit tests covering truncation warnings and deterministic ordering (`OS-OSQ-011`, `OS-OSQ-012`).
- [x] 4.3 Implement Bubble Tea interactive viewer (`cmd/remotr/query_viewer.go`) for `remotr query results --tui` supporting live `/` filter, column sorting, raw/grouped/endpoint view switching, and export (`OS-OSQ-011`).

## 5. Qualification, Traceability, Docs, and Verification

- [x] 5.1 Add container/VM provider contract tests for `osquery` private/global/existing modes (verifying non-root user cannot execute private install and Elastic Agent-style existing binary is never modified) and e2e sync/query tests (`OS-OSQ-003`, `OS-OSQ-005`, `OS-OSQ-007`).
- [x] 5.2 Update `test/traceability.yaml` for `OS-OSQ-001` through `OS-OSQ-012`, document `kind: queries`, `kind: osquery`, and `remotr query` in `docs/reference/configuration-format.md` and an operator guide, and verify `openspec validate` and `make test`.
