CREATE TABLE IF NOT EXISTS secret_names (
    name TEXT PRIMARY KEY,
    next_version BIGINT NOT NULL DEFAULT 1 CHECK (next_version > 0),
    active_version BIGINT,
    activation_generation BIGINT NOT NULL DEFAULT 0 CHECK (activation_generation >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS secret_versions (
    name TEXT NOT NULL REFERENCES secret_names (name) ON DELETE RESTRICT,
    version BIGINT NOT NULL CHECK (version > 0),
    envelope_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by TEXT NOT NULL,
    activated_at TIMESTAMPTZ,
    activated_by TEXT NOT NULL DEFAULT '',
    revoked_at TIMESTAMPTZ,
    revoked_by TEXT NOT NULL DEFAULT '',
    rollouts_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    PRIMARY KEY (name, version),
    CONSTRAINT secret_versions_revocation_actor CHECK (revoked_at IS NULL OR revoked_by <> '')
);

CREATE INDEX IF NOT EXISTS secret_versions_kek_idx
    ON secret_versions ((envelope_json->>'kekProvider'), (envelope_json->>'kekId'));
