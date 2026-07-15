-- name: AllocateSecretVersion :one
INSERT INTO secret_names (name, next_version)
VALUES ($1, 2)
ON CONFLICT (name) DO UPDATE
SET next_version = secret_names.next_version + 1,
    updated_at = now()
RETURNING next_version - 1 AS version;

-- name: CreateSecretVersion :exec
INSERT INTO secret_versions (name, version, envelope_json, created_at, created_by)
VALUES ($1, $2, $3, $4, $5);

-- name: GetExactSecretVersion :one
SELECT sv.*, (sn.active_version = sv.version) AS active,
       CASE WHEN sn.active_version = sv.version THEN sn.activation_generation ELSE 0 END AS activation_generation
FROM secret_versions sv
JOIN secret_names sn ON sn.name = sv.name
WHERE sv.name = $1 AND sv.version = $2;

-- name: GetActiveSecretVersion :one
SELECT sv.*, TRUE AS active, sn.activation_generation
FROM secret_versions sv
JOIN secret_names sn ON sn.name = sv.name AND sn.active_version = sv.version
WHERE sv.name = $1;

-- name: ListSecretVersions :many
SELECT sv.*, (sn.active_version = sv.version) AS active,
       CASE WHEN sn.active_version = sv.version THEN sn.activation_generation ELSE 0 END AS activation_generation
FROM secret_versions sv
JOIN secret_names sn ON sn.name = sv.name
WHERE sv.name = $1
ORDER BY sv.version;

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
SET revoked_at = COALESCE(revoked_at, now()),
    revoked_by = CASE WHEN revoked_at IS NULL THEN $3 ELSE revoked_by END
WHERE name = $1 AND version = $2
RETURNING sv.*,
    ((SELECT active_version FROM secret_names WHERE name = $1) = sv.version) AS active,
    CASE WHEN (SELECT active_version FROM secret_names WHERE name = $1) = sv.version
         THEN (SELECT activation_generation FROM secret_names WHERE name = $1)
         ELSE 0 END AS activation_generation;
