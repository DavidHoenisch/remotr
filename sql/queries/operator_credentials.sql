-- name: RegisterOperatorCredential :one
INSERT INTO operator_credentials (cert_fingerprint)
VALUES ($1)
ON CONFLICT (cert_fingerprint) DO UPDATE
    SET revoked_at = NULL
RETURNING *;

-- name: IsOperatorCredential :one
SELECT cert_fingerprint FROM operator_credentials
WHERE cert_fingerprint = $1
  AND revoked_at IS NULL;

-- name: ListOperatorCredentials :many
SELECT * FROM operator_credentials
WHERE revoked_at IS NULL
ORDER BY created_at;

-- name: CountOperatorCredentials :one
SELECT count(*)::bigint AS count FROM operator_credentials
WHERE revoked_at IS NULL;
