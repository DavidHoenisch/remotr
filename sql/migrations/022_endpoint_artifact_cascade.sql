-- Cached endpoint override artifacts are derived data: removing an endpoint
-- must remove them rather than fail with a foreign-key violation.
ALTER TABLE compiled_artifacts
    DROP CONSTRAINT IF EXISTS compiled_artifacts_endpoint_id_fkey,
    ADD CONSTRAINT compiled_artifacts_endpoint_id_fkey
        FOREIGN KEY (endpoint_id) REFERENCES endpoints (id) ON DELETE CASCADE;

ALTER TABLE compiled_artifact_variants
    DROP CONSTRAINT IF EXISTS compiled_artifact_variants_endpoint_id_fkey,
    ADD CONSTRAINT compiled_artifact_variants_endpoint_id_fkey
        FOREIGN KEY (endpoint_id) REFERENCES endpoints (id) ON DELETE CASCADE;
