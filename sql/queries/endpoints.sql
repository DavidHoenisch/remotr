-- name: RegisterEndpoint :one
INSERT INTO endpoints (id, fleet, cert_fingerprint)
VALUES ($1, $2, $3)
ON CONFLICT (id) DO UPDATE
    SET fleet = EXCLUDED.fleet,
        cert_fingerprint = COALESCE(EXCLUDED.cert_fingerprint, endpoints.cert_fingerprint),
        updated_at = now()
RETURNING *;

-- name: GetEndpointByID :one
SELECT * FROM endpoints
WHERE id = $1;

-- name: GetEndpointByFingerprint :one
SELECT * FROM endpoints
WHERE cert_fingerprint = $1;

-- name: BindFingerprint :one
UPDATE endpoints
SET cert_fingerprint = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ListEndpoints :many
SELECT * FROM endpoints
ORDER BY created_at;

-- name: DeleteEndpoint :execrows
DELETE FROM endpoints
WHERE id = $1;

-- name: UpdateEndpointCheckIn :exec
UPDATE endpoints
SET last_sync_at = now(),
    last_seen_release_ref = $2,
    last_seen_digest = $3,
    updated_at = now()
WHERE id = $1;
