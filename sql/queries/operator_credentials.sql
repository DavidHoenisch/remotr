-- name: RegisterOperatorCredential :one
INSERT INTO operator_credentials (cert_fingerprint, operator_id)
VALUES ($1, $2)
ON CONFLICT (cert_fingerprint) DO UPDATE
    SET revoked_at = NULL,
        operator_id = COALESCE(EXCLUDED.operator_id, operator_credentials.operator_id)
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
