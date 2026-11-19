-- name: UpsertQueryExecution :exec
INSERT INTO query_executions (
    run_id, endpoint_id, fleet, query_name, trigger_type, status,
    row_count, truncated, duration_ms, error_summary, scheduled_for, started_at, completed_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (run_id) DO UPDATE SET
    status        = EXCLUDED.status,
    row_count     = EXCLUDED.row_count,
    truncated     = EXCLUDED.truncated,
    duration_ms   = EXCLUDED.duration_ms,
    error_summary = EXCLUDED.error_summary,
    completed_at  = EXCLUDED.completed_at,
    updated_at    = EXCLUDED.updated_at;

-- name: ListLatestQueryExecutionsForEndpoint :many
SELECT DISTINCT ON (query_name)
    run_id, endpoint_id, fleet, query_name, trigger_type, status,
    row_count, truncated, duration_ms, error_summary, scheduled_for, started_at, completed_at, updated_at
FROM query_executions
WHERE endpoint_id = $1 AND trigger_type = 'scheduled'
ORDER BY query_name, started_at DESC;

-- name: ListLatestQueryExecutionsByFleetAndQuery :many
SELECT DISTINCT ON (endpoint_id)
    run_id, endpoint_id, fleet, query_name, trigger_type, status,
    row_count, truncated, duration_ms, error_summary, scheduled_for, started_at, completed_at, updated_at
FROM query_executions
WHERE ($1::text = '' OR fleet = $1) AND query_name = $2
ORDER BY endpoint_id, started_at DESC;

-- name: GetQuerySnapshot :one
SELECT endpoint_id, query_name, fleet, run_id, rows_json, row_count, truncated, updated_at
FROM query_snapshots
WHERE endpoint_id = $1 AND query_name = $2;

-- name: UpsertQuerySnapshot :exec
INSERT INTO query_snapshots (endpoint_id, query_name, fleet, run_id, rows_json, row_count, truncated, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (endpoint_id, query_name) DO UPDATE SET
    fleet      = EXCLUDED.fleet,
    run_id     = EXCLUDED.run_id,
    rows_json  = EXCLUDED.rows_json,
    row_count  = EXCLUDED.row_count,
    truncated  = EXCLUDED.truncated,
    updated_at = EXCLUDED.updated_at;

-- name: ListQuerySnapshots :many
SELECT endpoint_id, query_name, fleet, run_id, rows_json, row_count, truncated, updated_at
FROM query_snapshots
WHERE query_name = $1
  AND ($2::text = '' OR fleet = $2)
  AND ($3::text = '' OR endpoint_id = $3)
ORDER BY endpoint_id;

-- name: InsertQueryDiff :exec
INSERT INTO query_diffs (run_id, endpoint_id, fleet, query_name, added_rows, removed_rows, added_count, removed_count, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (run_id) DO NOTHING;

-- name: PruneQueryDiffs :exec
DELETE FROM query_diffs
WHERE run_id IN (
    SELECT d.run_id FROM query_diffs d
    WHERE d.endpoint_id = $1 AND d.query_name = $2
    ORDER BY d.created_at DESC
    OFFSET $3
);

-- name: ListQueryDiffs :many
SELECT run_id, endpoint_id, fleet, query_name, added_rows, removed_rows, added_count, removed_count, created_at
FROM query_diffs
WHERE query_name = $1
  AND ($2::text = '' OR fleet = $2)
  AND ($3::text = '' OR endpoint_id = $3)
  AND created_at >= $4
ORDER BY created_at DESC
LIMIT $5;

-- name: InsertAdhocQueryRequest :exec
INSERT INTO query_adhoc_requests (
    request_id, actor, fleet, endpoint_id, query_name, sql, is_inline, timeout_sec, max_rows, created_at, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: GetAdhocQueryRequest :one
SELECT request_id, actor, fleet, endpoint_id, query_name, sql, is_inline, timeout_sec, max_rows, created_at, expires_at
FROM query_adhoc_requests
WHERE request_id = $1;

-- name: ListPendingAdhocQueryRequestsForEndpoint :many
SELECT r.request_id, r.actor, r.fleet, r.endpoint_id, r.query_name, r.sql, r.is_inline, r.timeout_sec, r.max_rows, r.created_at, r.expires_at
FROM query_adhoc_requests r
WHERE r.expires_at > $1
  AND (r.endpoint_id = $2 OR (r.endpoint_id = '' AND r.fleet = $3))
  AND NOT EXISTS (
      SELECT 1 FROM query_adhoc_results res
      WHERE res.request_id = r.request_id AND res.endpoint_id = $2
  )
ORDER BY r.created_at ASC
LIMIT 16;

-- name: UpsertAdhocQueryResult :exec
INSERT INTO query_adhoc_results (
    request_id, endpoint_id, status, rows_json, row_count, truncated, duration_ms, error_summary, completed_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (request_id, endpoint_id) DO UPDATE SET
    status        = EXCLUDED.status,
    rows_json     = EXCLUDED.rows_json,
    row_count     = EXCLUDED.row_count,
    truncated     = EXCLUDED.truncated,
    duration_ms   = EXCLUDED.duration_ms,
    error_summary = EXCLUDED.error_summary,
    completed_at  = EXCLUDED.completed_at;

-- name: ListAdhocQueryResults :many
SELECT request_id, endpoint_id, status, rows_json, row_count, truncated, duration_ms, error_summary, completed_at
FROM query_adhoc_results
WHERE request_id = $1
ORDER BY endpoint_id;

-- name: DeleteExpiredAdhocQueryRequests :exec
DELETE FROM query_adhoc_requests
WHERE expires_at < $1;
