CREATE TABLE IF NOT EXISTS endpoint_capability_documents (
    endpoint_id TEXT PRIMARY KEY REFERENCES endpoints (id) ON DELETE CASCADE,
    digest TEXT NOT NULL,
    canonical_document BYTEA NOT NULL,
    received_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS endpoint_capability_documents_received_at_idx
    ON endpoint_capability_documents (received_at DESC);
