CREATE TABLE IF NOT EXISTS firewall_audit_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    endpoint_id TEXT NOT NULL REFERENCES endpoints (id) ON DELETE CASCADE,
    digest TEXT,
    report_json JSONB NOT NULL,
    reported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS firewall_audit_endpoint_idx ON firewall_audit_reports (endpoint_id);
CREATE INDEX IF NOT EXISTS firewall_audit_reported_at_idx ON firewall_audit_reports (reported_at DESC);
