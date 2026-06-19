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
