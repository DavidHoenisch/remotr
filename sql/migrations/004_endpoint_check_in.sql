ALTER TABLE endpoints
    ADD COLUMN IF NOT EXISTS last_sync_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_seen_release_ref TEXT,
    ADD COLUMN IF NOT EXISTS last_seen_digest TEXT;
