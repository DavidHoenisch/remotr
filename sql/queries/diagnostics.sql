-- name: InsertDiagnosticRequest :one
INSERT INTO diagnostic_requests (
    id, endpoint_id, requested_by, status, spec_json, s3_key, expires_at
) VALUES (
    $1, $2, $3, 'pending', $4, $5, $6
)
RETURNING *;

-- name: GetDiagnosticRequest :one
SELECT * FROM diagnostic_requests WHERE id = $1;

-- name: GetActiveDiagnosticRequestForEndpoint :one
SELECT * FROM diagnostic_requests
WHERE endpoint_id = $1
  AND status IN ('pending', 'dispatched', 'running')
ORDER BY created_at DESC
LIMIT 1;

-- name: MarkDiagnosticRequestDispatched :one
UPDATE diagnostic_requests
SET status = 'dispatched',
    dispatched_at = now()
WHERE id = $1
  AND status = 'pending'
RETURNING *;

-- name: MarkDiagnosticRequestRunning :one
UPDATE diagnostic_requests
SET status = 'running'
WHERE id = $1
  AND status IN ('pending', 'dispatched')
RETURNING *;

-- name: CompleteDiagnosticRequest :one
UPDATE diagnostic_requests
SET status = $2,
    sha256 = $3,
    size_bytes = $4,
    error_message = $5,
    completed_at = now()
WHERE id = $1
RETURNING *;

-- name: ExpireDiagnosticRequests :exec
UPDATE diagnostic_requests
SET status = 'expired',
    completed_at = COALESCE(completed_at, now())
WHERE expires_at < now()
  AND status NOT IN ('expired');

-- name: DeleteExpiredDiagnosticRequests :many
DELETE FROM diagnostic_requests
WHERE expires_at < now()
  AND status IN ('ready', 'failed', 'expired')
RETURNING *;
