-- Migrate endpoint identifiers from UUID to TEXT so hostname-style slugs work.
-- Safe to re-run: skips when endpoints.id is already TEXT.

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'endpoints'
          AND column_name = 'id'
          AND udt_name = 'uuid'
    ) THEN
        ALTER TABLE endpoint_labels DROP CONSTRAINT IF EXISTS endpoint_labels_endpoint_id_fkey;
        ALTER TABLE drift_reports DROP CONSTRAINT IF EXISTS drift_reports_endpoint_id_fkey;
        ALTER TABLE apply_failures DROP CONSTRAINT IF EXISTS apply_failures_endpoint_id_fkey;

        ALTER TABLE endpoints ALTER COLUMN id TYPE TEXT USING id::text;
        ALTER TABLE endpoint_labels ALTER COLUMN endpoint_id TYPE TEXT USING endpoint_id::text;
        ALTER TABLE drift_reports ALTER COLUMN endpoint_id TYPE TEXT USING endpoint_id::text;
        ALTER TABLE apply_failures ALTER COLUMN endpoint_id TYPE TEXT USING endpoint_id::text;

        ALTER TABLE endpoint_labels
            ADD CONSTRAINT endpoint_labels_endpoint_id_fkey
            FOREIGN KEY (endpoint_id) REFERENCES endpoints (id) ON DELETE CASCADE;
        ALTER TABLE drift_reports
            ADD CONSTRAINT drift_reports_endpoint_id_fkey
            FOREIGN KEY (endpoint_id) REFERENCES endpoints (id) ON DELETE CASCADE;
        ALTER TABLE apply_failures
            ADD CONSTRAINT apply_failures_endpoint_id_fkey
            FOREIGN KEY (endpoint_id) REFERENCES endpoints (id) ON DELETE CASCADE;
    END IF;
END $$;
