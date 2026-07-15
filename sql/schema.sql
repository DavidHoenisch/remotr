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
    reported_usernames TEXT,
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
    operator_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

-- Existing deployments may have operator_credentials without operator_id (pre-RBAC).
ALTER TABLE operator_credentials
    ADD COLUMN IF NOT EXISTS operator_id TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS operator_credentials_operator_id_active_idx
    ON operator_credentials (operator_id)
    WHERE operator_id IS NOT NULL AND revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS rbac_roles (
    name TEXT PRIMARY KEY,
    description TEXT NOT NULL DEFAULT '',
    built_in BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS rbac_rules (
    id UUID PRIMARY KEY,
    role_name TEXT NOT NULL REFERENCES rbac_roles (name) ON DELETE CASCADE,
    method TEXT NOT NULL,
    path_pattern TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (role_name, method, path_pattern)
);

CREATE TABLE IF NOT EXISTS operator_role_assignments (
    operator_id TEXT NOT NULL,
    role_name TEXT NOT NULL REFERENCES rbac_roles (name) ON DELETE CASCADE,
    PRIMARY KEY (operator_id, role_name)
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

CREATE TABLE IF NOT EXISTS endpoint_system_info (
    endpoint_id TEXT PRIMARY KEY REFERENCES endpoints (id) ON DELETE CASCADE,
    digest TEXT NOT NULL DEFAULT '',
    info_json JSONB NOT NULL,
    reported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS firewall_audit_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    endpoint_id TEXT NOT NULL REFERENCES endpoints (id) ON DELETE CASCADE,
    digest TEXT,
    report_json JSONB NOT NULL,
    reported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS firewall_audit_endpoint_idx ON firewall_audit_reports (endpoint_id);
CREATE INDEX IF NOT EXISTS firewall_audit_reported_at_idx ON firewall_audit_reports (reported_at DESC);

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

CREATE TABLE IF NOT EXISTS cron_last_run (
    endpoint_id TEXT NOT NULL REFERENCES endpoints (id) ON DELETE CASCADE,
    cron_name TEXT NOT NULL,
    crons_digest TEXT NOT NULL DEFAULT '',
    run_id UUID NOT NULL,
    scheduled_for TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL
        CHECK (status IN ('running', 'success', 'failed')),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    message TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (endpoint_id, cron_name)
);

CREATE INDEX IF NOT EXISTS cron_last_run_status_idx
    ON cron_last_run (status, updated_at DESC);

CREATE TABLE IF NOT EXISTS cron_executions (
    id UUID PRIMARY KEY,
    endpoint_id TEXT NOT NULL REFERENCES endpoints (id) ON DELETE CASCADE,
    cron_name TEXT NOT NULL,
    crons_digest TEXT NOT NULL DEFAULT '',
    release_ref TEXT NOT NULL,
    run_id UUID NOT NULL,
    scheduled_for TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    status TEXT NOT NULL
        CHECK (status IN ('running', 'success', 'failed')),
    message TEXT NOT NULL DEFAULT '',
    details_json JSONB,
    reported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS cron_executions_endpoint_idx
    ON cron_executions (endpoint_id, reported_at DESC);

CREATE INDEX IF NOT EXISTS cron_executions_run_idx
    ON cron_executions (run_id);

CREATE TABLE IF NOT EXISTS app_packages (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    s3_key TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    manifest JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (name, version)
);

CREATE INDEX IF NOT EXISTS app_packages_name_idx ON app_packages (name);

ALTER TABLE endpoints
    ADD COLUMN IF NOT EXISTS reported_usernames TEXT;

CREATE TABLE IF NOT EXISTS diagnostic_requests (
    id UUID PRIMARY KEY,
    endpoint_id TEXT NOT NULL REFERENCES endpoints (id) ON DELETE CASCADE,
    requested_by TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'dispatched', 'running', 'ready', 'failed', 'expired')),
    spec_json JSONB NOT NULL,
    s3_key TEXT NOT NULL DEFAULT '',
    sha256 TEXT NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    dispatched_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS diagnostic_requests_endpoint_status_idx
    ON diagnostic_requests (endpoint_id, status);

CREATE INDEX IF NOT EXISTS diagnostic_requests_expires_idx
    ON diagnostic_requests (expires_at)
    WHERE status IN ('ready', 'failed', 'expired');

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
