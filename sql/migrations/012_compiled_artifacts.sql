CREATE TABLE IF NOT EXISTS compiled_artifacts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fleet_name      TEXT,
    endpoint_id     TEXT REFERENCES endpoints (id),
    release_ref     TEXT NOT NULL,
    artifact_type   TEXT NOT NULL CHECK (artifact_type IN ('desired', 'crons')),
    artifact        BYTEA NOT NULL,
    digest          TEXT NOT NULL,
    compiled_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT compiled_artifacts_fleet_or_endpoint CHECK (
        (fleet_name IS NOT NULL AND endpoint_id IS NULL) OR
        (fleet_name IS NULL AND endpoint_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS compiled_artifacts_fleet_unique
    ON compiled_artifacts (fleet_name, release_ref, artifact_type)
    WHERE fleet_name IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS compiled_artifacts_endpoint_unique
    ON compiled_artifacts (endpoint_id, release_ref, artifact_type)
    WHERE endpoint_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS compiled_artifacts_fleet_idx
    ON compiled_artifacts (fleet_name, release_ref)
    WHERE fleet_name IS NOT NULL;

CREATE INDEX IF NOT EXISTS compiled_artifacts_endpoint_idx
    ON compiled_artifacts (endpoint_id, release_ref)
    WHERE endpoint_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS compiled_artifacts_compiled_at_idx
    ON compiled_artifacts (compiled_at);
