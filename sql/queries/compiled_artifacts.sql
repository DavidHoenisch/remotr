-- name: UpsertCompiledArtifactForFleet :one
INSERT INTO compiled_artifacts (
    fleet_name,
    endpoint_id,
    release_ref,
    artifact_type,
    artifact,
    digest,
    compiled_at
) VALUES (
    $1,
    NULL,
    $2,
    $3,
    $4,
    $5,
    NOW()
)
ON CONFLICT (fleet_name, release_ref, artifact_type)
    WHERE fleet_name IS NOT NULL
DO UPDATE SET
    artifact = EXCLUDED.artifact,
    digest = EXCLUDED.digest,
    compiled_at = NOW()
RETURNING *;

-- name: UpsertCompiledArtifactForEndpoint :one
INSERT INTO compiled_artifacts (
    fleet_name,
    endpoint_id,
    release_ref,
    artifact_type,
    artifact,
    digest,
    compiled_at
) VALUES (
    NULL,
    $1,
    $2,
    $3,
    $4,
    $5,
    NOW()
)
ON CONFLICT (endpoint_id, release_ref, artifact_type)
    WHERE endpoint_id IS NOT NULL
DO UPDATE SET
    artifact = EXCLUDED.artifact,
    digest = EXCLUDED.digest,
    compiled_at = NOW()
RETURNING *;

-- name: GetCompiledArtifactForFleet :one
SELECT artifact, digest
FROM compiled_artifacts
WHERE fleet_name = $1
  AND release_ref = $2
  AND artifact_type = $3;

-- name: GetCompiledArtifactForEndpoint :one
SELECT artifact, digest
FROM compiled_artifacts
WHERE endpoint_id = $1
  AND release_ref = $2
  AND artifact_type = $3;

-- name: PruneOldCompiledArtifacts :exec
DELETE FROM compiled_artifacts
WHERE compiled_at < $1;
