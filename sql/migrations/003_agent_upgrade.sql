-- Per-endpoint agent upgrade intent (operator "taint"); cleared when reported version matches.

ALTER TABLE endpoints
    ADD COLUMN IF NOT EXISTS desired_agent_version TEXT,
    ADD COLUMN IF NOT EXISTS desired_agent_version_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS reported_agent_version TEXT,
    ADD COLUMN IF NOT EXISTS agent_upgrade_phase TEXT,
    ADD COLUMN IF NOT EXISTS agent_upgrade_message TEXT,
    ADD COLUMN IF NOT EXISTS agent_upgrade_reported_at TIMESTAMPTZ;
