-- Remotr server registry DDL (migrations-friendly: one statement per object).
-- Apply: psql "$DATABASE_URL" -f sql/schema.sql
-- Or: docker compose -f compose/docker-compose.yml exec -T postgres \
--       psql -U remotr -d remotr -f - < sql/schema.sql

CREATE TABLE IF NOT EXISTS fleet_settings (
    fleet TEXT PRIMARY KEY,
    remediation_policy TEXT NOT NULL DEFAULT 'auto'
        CHECK (remediation_policy IN ('auto', 'report')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS endpoints (
    id TEXT PRIMARY KEY
        CHECK (char_length(id) >= 4 AND char_length(id) <= 63 AND id ~ '^[a-zA-Z0-9-]+$'),
    fleet TEXT NOT NULL REFERENCES fleet_settings (fleet),
    cert_fingerprint TEXT UNIQUE,
    desired_agent_version TEXT,
    desired_agent_version_at TIMESTAMPTZ,
    reported_agent_version TEXT,
    agent_upgrade_phase TEXT,
    agent_upgrade_message TEXT,
    agent_upgrade_reported_at TIMESTAMPTZ,
    last_sync_at TIMESTAMPTZ,
    last_seen_release_ref TEXT,
    last_seen_digest TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS endpoints_fleet_idx ON endpoints (fleet);

CREATE TABLE IF NOT EXISTS enrollment_tokens (
    token TEXT PRIMARY KEY,
    fleet TEXT NOT NULL REFERENCES fleet_settings (fleet),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS enrollment_tokens_active_idx
    ON enrollment_tokens (fleet)
    WHERE consumed_at IS NULL AND revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS deployment_tokens (
    id UUID PRIMARY KEY,
    label TEXT NOT NULL UNIQUE,
    fleet TEXT NOT NULL REFERENCES fleet_settings (fleet),
    secret_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS deployment_tokens_fleet_idx ON deployment_tokens (fleet);
CREATE INDEX IF NOT EXISTS deployment_tokens_active_idx
    ON deployment_tokens (label)
    WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS operator_credentials (
    cert_fingerprint TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS endpoint_labels (
    endpoint_id TEXT NOT NULL REFERENCES endpoints (id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (endpoint_id, key)
);

CREATE TABLE IF NOT EXISTS drift_reports (
    id UUID PRIMARY KEY,
    endpoint_id TEXT NOT NULL REFERENCES endpoints (id) ON DELETE CASCADE,
    release_ref TEXT NOT NULL,
    digest TEXT NOT NULL,
    report_json JSONB NOT NULL,
    reported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS drift_reports_endpoint_idx
    ON drift_reports (endpoint_id, reported_at DESC);

CREATE TABLE IF NOT EXISTS apply_failures (
    id UUID PRIMARY KEY,
    endpoint_id TEXT NOT NULL REFERENCES endpoints (id) ON DELETE CASCADE,
    release_ref TEXT NOT NULL,
    resource_address TEXT NOT NULL,
    message TEXT NOT NULL,
    reported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS apply_failures_endpoint_idx
    ON apply_failures (endpoint_id, reported_at DESC);

CREATE TABLE IF NOT EXISTS server_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_events (
    id UUID PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    request_id TEXT,
    actor_type TEXT NOT NULL
        CHECK (actor_type IN ('operator', 'endpoint', 'anonymous', 'system')),
    actor_id TEXT,
    actor_fingerprint TEXT,
    action TEXT NOT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    status_code INT NOT NULL,
    resource_type TEXT,
    resource_id TEXT,
    client_ip TEXT,
    details JSONB
);

CREATE INDEX IF NOT EXISTS audit_events_occurred_at_idx
    ON audit_events (occurred_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS audit_events_action_idx
    ON audit_events (action, occurred_at DESC);
