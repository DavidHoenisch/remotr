-- name: AllocateSecretVersion :one
INSERT INTO secret_names (name, scope_type, scope_id, next_version)
VALUES ($1, $2, NULLIF($3, ''), 2)
ON CONFLICT (name) DO UPDATE
SET next_version = secret_names.next_version + 1,
    updated_at = now()
WHERE secret_names.scope_type = EXCLUDED.scope_type
  AND secret_names.scope_id IS NOT DISTINCT FROM EXCLUDED.scope_id
RETURNING (next_version - 1)::BIGINT AS version;

-- name: CreateSecretVersion :exec
INSERT INTO secret_versions (name, version, envelope_json, created_at, created_by)
VALUES ($1, $2, $3, $4, $5);

-- name: GetExactSecretVersion :one
SELECT sv.*, CASE WHEN sn.active_version = sv.version THEN TRUE ELSE FALSE END AS active,
       CASE WHEN sn.active_version = sv.version THEN sn.activation_generation ELSE 0::BIGINT END AS activation_generation
FROM secret_versions sv
JOIN secret_names sn ON sn.name = sv.name
WHERE sv.name = $1 AND sv.version = $2;

-- name: GetActiveSecretVersion :one
SELECT sv.*, TRUE AS active, sn.activation_generation
FROM secret_versions sv
JOIN secret_names sn ON sn.name = sv.name AND sn.active_version = sv.version
WHERE sv.name = $1;

-- name: ListSecretVersions :many
SELECT sv.*, CASE WHEN sn.active_version = sv.version THEN TRUE ELSE FALSE END AS active,
       CASE WHEN sn.active_version = sv.version THEN sn.activation_generation ELSE 0::BIGINT END AS activation_generation
FROM secret_versions sv
JOIN secret_names sn ON sn.name = sv.name
WHERE sv.name = $1
ORDER BY sv.version;

-- name: ListLogicalSecrets :many
SELECT sn.name, sn.scope_type, COALESCE(sn.scope_id, '') AS scope_id,
       COALESCE(sn.active_version, 0)::BIGINT AS active_version,
       count(sv.version)::BIGINT AS version_count,
       COALESCE(active.envelope_json->>'fingerprint', '') AS fingerprint,
       min(sv.created_at) AS created_at,
       max(sv.created_at) AS updated_at
FROM secret_names sn
JOIN secret_versions sv ON sv.name = sn.name
LEFT JOIN secret_versions active ON active.name = sn.name AND active.version = sn.active_version
WHERE sn.name > sqlc.arg(cursor)
GROUP BY sn.name, sn.scope_type, sn.scope_id, sn.active_version, active.envelope_json
ORDER BY sn.name
LIMIT sqlc.arg(page_size);

-- name: GetSecretActivationGeneration :one
SELECT activation_generation FROM secret_names WHERE name = $1;

-- name: ActivateSecretVersion :one
WITH candidate AS (
    SELECT 1 FROM secret_versions
    WHERE name = $1 AND version = $2 AND revoked_at IS NULL
), activated_name AS (
    UPDATE secret_names
    SET active_version = $2,
        activation_generation = activation_generation + 1,
        updated_at = now()
    WHERE name = $1
      AND activation_generation = $3 - 1
      AND EXISTS (SELECT 1 FROM candidate)
    RETURNING activation_generation
)
UPDATE secret_versions sv
SET activated_at = now(), activated_by = $4, rollouts_json = $5
FROM activated_name an
WHERE sv.name = $1 AND sv.version = $2
RETURNING sv.*, TRUE AS active, an.activation_generation;

-- name: RevokeSecretVersion :one
UPDATE secret_versions sv
SET revoked_at = COALESCE(sv.revoked_at, now()),
    revoked_by = CASE WHEN sv.revoked_at IS NULL THEN $3 ELSE sv.revoked_by END
WHERE sv.name = $1 AND sv.version = $2
RETURNING sv.*,
    CASE WHEN (SELECT active_version FROM secret_names WHERE secret_names.name = $1) = sv.version
         THEN TRUE ELSE FALSE END AS active,
    CASE WHEN (SELECT active_version FROM secret_names WHERE secret_names.name = $1) = sv.version
         THEN (SELECT activation_generation FROM secret_names WHERE secret_names.name = $1)
         ELSE 0::BIGINT END AS activation_generation;

-- name: CreateSecretRollbackReference :exec
INSERT INTO secret_rollback_references (
    id, name, version, fingerprint, resource_address, artifact_digest,
    attempt, created_at, expires_at, status
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: ListActiveSecretRollbackReferences :many
SELECT id, name, version, fingerprint, resource_address, artifact_digest,
       attempt, created_at, expires_at, status, abandoned_at, abandoned_by
FROM secret_rollback_references
WHERE name = $1 AND version = $2 AND status = 'armed' AND expires_at > sqlc.arg(now)
ORDER BY id;

-- name: AbandonSecretRollbackReferences :exec
UPDATE secret_rollback_references
SET status = 'abandoned', abandoned_at = $4, abandoned_by = $3
WHERE name = $1 AND version = $2 AND status = 'armed' AND expires_at > $4;

-- name: DeleteSecretVersion :one
DELETE FROM secret_versions sv
WHERE sv.name = $1 AND sv.version = $2
  AND NOT EXISTS (
      SELECT 1 FROM secret_names sn
      WHERE sn.name = sv.name AND sn.active_version = sv.version
  )
  AND NOT EXISTS (
      SELECT 1 FROM secret_rollback_references rr
      WHERE rr.name = sv.name AND rr.version = sv.version
        AND rr.status = 'armed' AND rr.expires_at > sqlc.arg(now)
  )
RETURNING sv.version;
