-- Endpoint reservations: an operator may reserve an endpoint ID before any
-- machine enrolls. A reserved endpoint has no certificate and is claimed by a
-- single-use enrollment token bound to that ID.
ALTER TABLE endpoints
    ADD COLUMN IF NOT EXISTS enrollment_state TEXT NOT NULL DEFAULT 'enrolled';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'endpoints_enrollment_state_check'
    ) THEN
        ALTER TABLE endpoints
            ADD CONSTRAINT endpoints_enrollment_state_check
            CHECK (enrollment_state IN ('reserved', 'enrolled'));
    END IF;
END $$;

ALTER TABLE enrollment_tokens
    ADD COLUMN IF NOT EXISTS endpoint_id TEXT REFERENCES endpoints (id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS enrollment_tokens_endpoint_idx
    ON enrollment_tokens (endpoint_id)
    WHERE endpoint_id IS NOT NULL;
