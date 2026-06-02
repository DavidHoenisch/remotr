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
  AND consumed_at IS NULL
  AND revoked_at IS NULL
  AND expires_at > now()
RETURNING *;
