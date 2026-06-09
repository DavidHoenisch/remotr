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
