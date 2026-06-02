-- name: CreateDeploymentToken :one
INSERT INTO deployment_tokens (id, label, fleet, secret_hash, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, label, fleet, secret_hash, expires_at, revoked_at, created_at, last_used_at;

-- name: ListDeploymentTokens :many
SELECT id, label, fleet, expires_at, revoked_at, created_at, last_used_at
FROM deployment_tokens
ORDER BY created_at;

-- name: GetDeploymentTokenByLabel :one
SELECT id, label, fleet, secret_hash, expires_at, revoked_at, created_at, last_used_at
FROM deployment_tokens
WHERE label = $1;

-- name: GetDeploymentTokenByID :one
SELECT id, label, fleet, secret_hash, expires_at, revoked_at, created_at, last_used_at
FROM deployment_tokens
WHERE id = $1;

-- name: RevokeDeploymentToken :execrows
UPDATE deployment_tokens
SET revoked_at = now()
WHERE label = $1
  AND revoked_at IS NULL;

-- name: TouchDeploymentTokenUsed :exec
UPDATE deployment_tokens
SET last_used_at = now()
WHERE id = $1;
