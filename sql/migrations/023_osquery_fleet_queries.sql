-- Osquery fleet queries: compiled artifact support, executions, latest
-- snapshots, server-side row diffs, and TTL-bounded ad-hoc live queries.

ALTER TABLE compiled_artifacts
    DROP CONSTRAINT IF EXISTS compiled_artifacts_artifact_type_check,
    ADD CONSTRAINT compiled_artifacts_artifact_type_check
        CHECK (artifact_type IN ('desired', 'crons', 'queries'));

CREATE TABLE IF NOT EXISTS query_executions (
    run_id        TEXT PRIMARY KEY,
    endpoint_id   TEXT NOT NULL REFERENCES endpoints (id) ON DELETE CASCADE,
    fleet         TEXT NOT NULL,
    query_name    TEXT NOT NULL,
    trigger_type  TEXT NOT NULL CHECK (trigger_type IN ('scheduled', 'adhoc')),
    status        TEXT NOT NULL CHECK (status IN ('running', 'success', 'failed', 'unsupported')),
    row_count     INTEGER NOT NULL DEFAULT 0,
    truncated     BOOLEAN NOT NULL DEFAULT FALSE,
    duration_ms   BIGINT NOT NULL DEFAULT 0,
    error_summary TEXT NOT NULL DEFAULT '',
    scheduled_for TIMESTAMPTZ,
    started_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at  TIMESTAMPTZ,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS query_executions_fleet_query_idx
    ON query_executions (fleet, query_name, completed_at DESC);

CREATE INDEX IF NOT EXISTS query_executions_endpoint_query_idx
    ON query_executions (endpoint_id, query_name, started_at DESC);

CREATE TABLE IF NOT EXISTS query_snapshots (
    endpoint_id TEXT NOT NULL REFERENCES endpoints (id) ON DELETE CASCADE,
    query_name  TEXT NOT NULL,
    fleet       TEXT NOT NULL,
    run_id      TEXT NOT NULL,
    rows_json   JSONB NOT NULL DEFAULT '[]'::jsonb,
    row_count   INTEGER NOT NULL DEFAULT 0,
    truncated   BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (endpoint_id, query_name)
);

CREATE INDEX IF NOT EXISTS query_snapshots_fleet_query_idx
    ON query_snapshots (fleet, query_name, updated_at DESC);

CREATE TABLE IF NOT EXISTS query_diffs (
    run_id        TEXT PRIMARY KEY,
    endpoint_id   TEXT NOT NULL REFERENCES endpoints (id) ON DELETE CASCADE,
    fleet         TEXT NOT NULL,
    query_name    TEXT NOT NULL,
    added_rows    JSONB NOT NULL DEFAULT '[]'::jsonb,
    removed_rows  JSONB NOT NULL DEFAULT '[]'::jsonb,
    added_count   INTEGER NOT NULL DEFAULT 0,
    removed_count INTEGER NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS query_diffs_endpoint_query_idx
    ON query_diffs (endpoint_id, query_name, created_at DESC);

CREATE INDEX IF NOT EXISTS query_diffs_fleet_query_idx
    ON query_diffs (fleet, query_name, created_at DESC);

CREATE TABLE IF NOT EXISTS query_adhoc_requests (
    request_id   TEXT PRIMARY KEY,
    actor        TEXT NOT NULL,
    fleet        TEXT NOT NULL DEFAULT '',
    endpoint_id  TEXT NOT NULL DEFAULT '',
    query_name   TEXT NOT NULL DEFAULT '',
    sql          TEXT NOT NULL,
    is_inline    BOOLEAN NOT NULL DEFAULT FALSE,
    timeout_sec  INTEGER NOT NULL DEFAULT 15,
    max_rows     INTEGER NOT NULL DEFAULT 500,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS query_adhoc_requests_expires_idx
    ON query_adhoc_requests (expires_at);

CREATE TABLE IF NOT EXISTS query_adhoc_results (
    request_id    TEXT NOT NULL REFERENCES query_adhoc_requests (request_id) ON DELETE CASCADE,
    endpoint_id   TEXT NOT NULL REFERENCES endpoints (id) ON DELETE CASCADE,
    status        TEXT NOT NULL CHECK (status IN ('running', 'success', 'failed', 'unsupported')),
    rows_json     JSONB NOT NULL DEFAULT '[]'::jsonb,
    row_count     INTEGER NOT NULL DEFAULT 0,
    truncated     BOOLEAN NOT NULL DEFAULT FALSE,
    duration_ms   BIGINT NOT NULL DEFAULT 0,
    error_summary TEXT NOT NULL DEFAULT '',
    completed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (request_id, endpoint_id)
);
