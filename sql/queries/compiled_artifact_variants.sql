-- name: UpsertCompiledArtifactVariantForFleet :one
INSERT INTO compiled_artifact_variants (
    fleet_name, endpoint_id, release_ref, artifact_type, schema_version,
    source_digest, requirement_set_digest, requirement_set, artifact, digest, compiled_at
) VALUES (
    $1, NULL, $2, $3, $4, $5, $6, $7, $8, $9, NOW()
)
ON CONFLICT (fleet_name, release_ref, artifact_type, schema_version, requirement_set_digest)
    WHERE fleet_name IS NOT NULL
DO UPDATE SET
    source_digest = EXCLUDED.source_digest,
    requirement_set = EXCLUDED.requirement_set,
    artifact = EXCLUDED.artifact,
    digest = EXCLUDED.digest,
    compiled_at = NOW()
RETURNING *;

-- name: UpsertCompiledArtifactVariantForEndpoint :one
INSERT INTO compiled_artifact_variants (
    fleet_name, endpoint_id, release_ref, artifact_type, schema_version,
    source_digest, requirement_set_digest, requirement_set, artifact, digest, compiled_at
) VALUES (
    NULL, $1, $2, $3, $4, $5, $6, $7, $8, $9, NOW()
)
ON CONFLICT (endpoint_id, release_ref, artifact_type, schema_version, requirement_set_digest)
    WHERE endpoint_id IS NOT NULL
DO UPDATE SET
    source_digest = EXCLUDED.source_digest,
    requirement_set = EXCLUDED.requirement_set,
    artifact = EXCLUDED.artifact,
    digest = EXCLUDED.digest,
    compiled_at = NOW()
RETURNING *;

-- name: ListCompiledArtifactVariantsForFleet :many
SELECT * FROM compiled_artifact_variants
WHERE fleet_name = $1 AND release_ref = $2 AND artifact_type = $3
ORDER BY schema_version DESC, requirement_set_digest;

-- name: ListCompiledArtifactVariantsForEndpoint :many
SELECT * FROM compiled_artifact_variants
WHERE endpoint_id = $1 AND release_ref = $2 AND artifact_type = $3
ORDER BY schema_version DESC, requirement_set_digest;

-- name: PruneOldCompiledArtifactVariants :exec
DELETE FROM compiled_artifact_variants WHERE compiled_at < $1;
