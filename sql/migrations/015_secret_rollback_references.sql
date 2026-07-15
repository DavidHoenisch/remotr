CREATE TABLE IF NOT EXISTS secret_rollback_references (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    version BIGINT NOT NULL,
    fingerprint TEXT NOT NULL,
    resource_address TEXT NOT NULL,
    artifact_digest TEXT NOT NULL,
    attempt BIGINT NOT NULL CHECK (attempt > 0),
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('armed', 'completed', 'abandoned')),
    abandoned_at TIMESTAMPTZ,
    abandoned_by TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (name, version) REFERENCES secret_versions (name, version) ON DELETE CASCADE,
    CONSTRAINT secret_rollback_reference_expiry CHECK (expires_at > created_at),
    CONSTRAINT secret_rollback_reference_abandonment CHECK (
        (status = 'abandoned' AND abandoned_at IS NOT NULL AND abandoned_by <> '') OR
        (status <> 'abandoned' AND abandoned_at IS NULL AND abandoned_by = '')
    )
);

CREATE INDEX IF NOT EXISTS secret_rollback_references_version_idx
    ON secret_rollback_references (name, version, status, expires_at);
