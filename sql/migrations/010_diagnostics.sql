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
