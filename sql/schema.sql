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
    id UUID PRIMARY KEY,
    fleet TEXT NOT NULL REFERENCES fleet_settings (fleet),
    cert_fingerprint TEXT UNIQUE,
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

CREATE TABLE IF NOT EXISTS operator_credentials (
    cert_fingerprint TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS endpoint_labels (
    endpoint_id UUID NOT NULL REFERENCES endpoints (id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (endpoint_id, key)
);

CREATE TABLE IF NOT EXISTS drift_reports (
    id UUID PRIMARY KEY,
    endpoint_id UUID NOT NULL REFERENCES endpoints (id) ON DELETE CASCADE,
    release_ref TEXT NOT NULL,
    digest TEXT NOT NULL,
    report_json JSONB NOT NULL,
    reported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS drift_reports_endpoint_idx
    ON drift_reports (endpoint_id, reported_at DESC);

CREATE TABLE IF NOT EXISTS apply_failures (
    id UUID PRIMARY KEY,
    endpoint_id UUID NOT NULL REFERENCES endpoints (id) ON DELETE CASCADE,
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
