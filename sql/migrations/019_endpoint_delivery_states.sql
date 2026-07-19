CREATE TABLE IF NOT EXISTS endpoint_delivery_states (
    endpoint_id                   TEXT PRIMARY KEY REFERENCES endpoints (id) ON DELETE CASCADE,
    target_release_ref            TEXT NOT NULL DEFAULT '',
    offered_release_ref           TEXT NOT NULL DEFAULT '',
    offered_digest                TEXT NOT NULL DEFAULT '',
    offered_schema_version        INTEGER NOT NULL DEFAULT 0 CHECK (offered_schema_version IN (0, 1)),
    offered_at                    TIMESTAMPTZ,
    active_release_ref            TEXT NOT NULL DEFAULT '',
    active_digest                 TEXT NOT NULL DEFAULT '',
    active_schema_version         INTEGER NOT NULL DEFAULT 0 CHECK (active_schema_version IN (0, 1)),
    active_at                     TIMESTAMPTZ,
    capability_blocked_target_ref TEXT NOT NULL DEFAULT '',
    missing_requirements          JSONB NOT NULL DEFAULT '[]'::jsonb,
    unmanaged                     BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at                    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS endpoint_delivery_states_blocked_idx
    ON endpoint_delivery_states (capability_blocked_target_ref)
    WHERE capability_blocked_target_ref <> '';
