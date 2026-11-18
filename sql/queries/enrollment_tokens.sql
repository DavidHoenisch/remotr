-- name: CreateEnrollmentToken :one
INSERT INTO enrollment_tokens (token, fleet, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListEnrollmentTokens :many
SELECT * FROM enrollment_tokens
WHERE consumed_at IS NULL
  AND revoked_at IS NULL
ORDER BY created_at;

-- name: RevokeEnrollmentToken :execrows
UPDATE enrollment_tokens
SET revoked_at = now()
WHERE token = $1
  AND revoked_at IS NULL
  AND consumed_at IS NULL;

-- name: ConsumeEnrollmentToken :one
UPDATE enrollment_tokens
SET consumed_at = now()
WHERE token = $1
  AND endpoint_id IS NULL
  AND consumed_at IS NULL
  AND revoked_at IS NULL
  AND expires_at > now()
RETURNING *;

-- name: CreateBoundEnrollmentToken :one
INSERT INTO enrollment_tokens (token, fleet, expires_at, endpoint_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: RevokeBoundEnrollmentTokens :execrows
UPDATE enrollment_tokens
SET revoked_at = now()
WHERE endpoint_id = $1
  AND revoked_at IS NULL
  AND consumed_at IS NULL;

-- name: GetEnrollmentTokenBinding :one
SELECT endpoint_id FROM enrollment_tokens
WHERE token = $1
  AND endpoint_id IS NOT NULL;

-- name: ConsumeBoundEnrollmentToken :one
UPDATE enrollment_tokens
SET consumed_at = now()
WHERE token = $1
  AND endpoint_id = $2
  AND consumed_at IS NULL
  AND revoked_at IS NULL
  AND expires_at > now()
RETURNING *;
