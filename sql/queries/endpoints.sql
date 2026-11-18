-- name: RegisterEndpoint :one
INSERT INTO endpoints (id, fleet, cert_fingerprint)
VALUES ($1, $2, $3)
ON CONFLICT (id) DO NOTHING
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

-- name: ReassignEndpoint :execrows
UPDATE endpoints
SET fleet = $2,
    updated_at = now()
WHERE id = $1;

-- name: UpdateEndpointCheckIn :exec
UPDATE endpoints
SET last_sync_at = now(),
    last_seen_release_ref = $2,
    last_seen_digest = $3,
    updated_at = now()
WHERE id = $1;

-- name: UpdateEndpointUsernames :exec
UPDATE endpoints
SET reported_usernames = $2,
    updated_at = now()
WHERE id = $1;

-- name: ReserveEndpoint :one
INSERT INTO endpoints (id, fleet, enrollment_state)
VALUES ($1, $2, 'reserved')
ON CONFLICT (id) DO UPDATE
SET updated_at = now()
WHERE endpoints.enrollment_state = 'reserved'
  AND endpoints.fleet = EXCLUDED.fleet
RETURNING *;

-- name: ClaimReservedEndpoint :one
UPDATE endpoints
SET enrollment_state = 'enrolled',
    cert_fingerprint = $3,
    updated_at = now()
WHERE id = $1
  AND fleet = $2
  AND enrollment_state = 'reserved'
RETURNING *;

-- name: DeleteReservedEndpoint :execrows
DELETE FROM endpoints
WHERE id = $1
  AND enrollment_state = 'reserved';
