CREATE TABLE IF NOT EXISTS compiled_artifact_variants (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fleet_name             TEXT,
    endpoint_id            TEXT REFERENCES endpoints (id),
    release_ref            TEXT NOT NULL,
    artifact_type          TEXT NOT NULL CHECK (artifact_type IN ('desired', 'crons')),
    schema_version         INTEGER NOT NULL CHECK (schema_version IN (0, 1)),
    source_digest          TEXT NOT NULL,
    requirement_set_digest TEXT NOT NULL,
    requirement_set        JSONB NOT NULL,
    artifact               BYTEA NOT NULL,
    digest                 TEXT NOT NULL,
    compiled_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT compiled_artifact_variants_fleet_or_endpoint CHECK (
        (fleet_name IS NOT NULL AND endpoint_id IS NULL) OR
        (fleet_name IS NULL AND endpoint_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS compiled_artifact_variants_fleet_unique
    ON compiled_artifact_variants (fleet_name, release_ref, artifact_type, schema_version, requirement_set_digest)
    WHERE fleet_name IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS compiled_artifact_variants_endpoint_unique
    ON compiled_artifact_variants (endpoint_id, release_ref, artifact_type, schema_version, requirement_set_digest)
    WHERE endpoint_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS compiled_artifact_variants_fleet_lookup
    ON compiled_artifact_variants (fleet_name, release_ref, artifact_type, schema_version DESC)
    WHERE fleet_name IS NOT NULL;

CREATE INDEX IF NOT EXISTS compiled_artifact_variants_endpoint_lookup
    ON compiled_artifact_variants (endpoint_id, release_ref, artifact_type, schema_version DESC)
    WHERE endpoint_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS compiled_artifact_variants_compiled_at_idx
    ON compiled_artifact_variants (compiled_at);
