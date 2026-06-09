-- name: ListAuditEvents :many
SELECT
    id, occurred_at, request_id, actor_type, actor_id, actor_fingerprint,
    action, method, path, status_code, resource_type, resource_id, client_ip, details
FROM audit_events
WHERE (sqlc.narg('since')::timestamptz IS NULL OR occurred_at >= sqlc.narg('since'))
  AND (sqlc.narg('until')::timestamptz IS NULL OR occurred_at <= sqlc.narg('until'))
  AND (sqlc.narg('action')::text IS NULL OR sqlc.narg('action') = '' OR action = sqlc.narg('action'))
  AND (sqlc.narg('actor_type')::text IS NULL OR sqlc.narg('actor_type') = '' OR actor_type = sqlc.narg('actor_type'))
  AND (
    sqlc.narg('cursor_at')::timestamptz IS NULL OR sqlc.narg('cursor_id')::uuid IS NULL
    OR (occurred_at, id) < (sqlc.narg('cursor_at'), sqlc.narg('cursor_id'))
  )
ORDER BY occurred_at DESC, id DESC
LIMIT sqlc.arg('limit');

-- name: InsertAuditEvent :exec
INSERT INTO audit_events (
    id, occurred_at, request_id, actor_type, actor_id, actor_fingerprint,
    action, method, path, status_code, resource_type, resource_id, client_ip, details
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
);
