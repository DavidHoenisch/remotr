-- name: GetCronLastRun :one
SELECT endpoint_id, cron_name, crons_digest, run_id, scheduled_for, status,
       started_at, completed_at, message, updated_at
FROM cron_last_run
WHERE endpoint_id = $1 AND cron_name = $2;

-- name: ListCronLastRunsForEndpoint :many
SELECT endpoint_id, cron_name, crons_digest, run_id, scheduled_for, status,
       started_at, completed_at, message, updated_at
FROM cron_last_run
WHERE endpoint_id = $1
ORDER BY cron_name;

-- name: UpsertCronLastRun :exec
INSERT INTO cron_last_run (
    endpoint_id, cron_name, crons_digest, run_id, scheduled_for, status,
    started_at, completed_at, message, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
ON CONFLICT (endpoint_id, cron_name) DO UPDATE SET
    crons_digest = EXCLUDED.crons_digest,
    run_id = EXCLUDED.run_id,
    scheduled_for = EXCLUDED.scheduled_for,
    status = EXCLUDED.status,
    started_at = EXCLUDED.started_at,
    completed_at = EXCLUDED.completed_at,
    message = EXCLUDED.message,
    updated_at = now();

-- name: InsertCronExecution :exec
INSERT INTO cron_executions (
    id, endpoint_id, cron_name, crons_digest, release_ref, run_id,
    scheduled_for, started_at, completed_at, status, message, details_json, reported_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, now());

-- name: ListCronExecutionsForEndpoint :many
SELECT id, endpoint_id, cron_name, crons_digest, release_ref, run_id,
       scheduled_for, started_at, completed_at, status, message, details_json, reported_at
FROM cron_executions
WHERE endpoint_id = $1
ORDER BY reported_at DESC
LIMIT $2;
