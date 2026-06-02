-- Enforce endpoint id rules in Postgres (matches identity.ValidateEndpointID).
-- Safe to re-run: skips when constraint already exists.

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'endpoints_id_format_check'
          AND conrelid = 'public.endpoints'::regclass
    ) THEN
        ALTER TABLE endpoints
            ADD CONSTRAINT endpoints_id_format_check
            CHECK (
                char_length(id) >= 4
                AND char_length(id) <= 63
                AND id ~ '^[a-zA-Z0-9-]+$'
            );
    END IF;
END $$;
