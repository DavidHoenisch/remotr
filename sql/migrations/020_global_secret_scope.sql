ALTER TABLE secret_names
    ADD COLUMN IF NOT EXISTS scope_type TEXT,
    ADD COLUMN IF NOT EXISTS scope_id TEXT;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM secret_versions sv
        CROSS JOIN LATERAL (
            SELECT
                NULLIF(sv.envelope_json #>> '{scope,scope}', '') AS scope_type,
                NULLIF(sv.envelope_json #>> '{scope,fleet}', '') AS fleet,
                NULLIF(sv.envelope_json #>> '{scope,endpointId}', '') AS endpoint_id
        ) s
        WHERE
            (s.scope_type IS NULL AND ((s.fleet IS NULL) = (s.endpoint_id IS NULL)))
            OR (s.scope_type IS NOT NULL AND s.scope_type NOT IN ('global', 'fleet', 'endpoint'))
            OR (s.scope_type = 'global' AND (s.fleet IS NOT NULL OR s.endpoint_id IS NOT NULL))
            OR (s.scope_type = 'fleet' AND (s.fleet IS NULL OR s.endpoint_id IS NOT NULL))
            OR (s.scope_type = 'endpoint' AND (s.endpoint_id IS NULL OR s.fleet IS NOT NULL))
            OR COALESCE(s.fleet <> btrim(s.fleet), FALSE)
            OR COALESCE(s.endpoint_id <> btrim(s.endpoint_id), FALSE)
    ) THEN
        RAISE EXCEPTION 'legacy secret version has neither or both scope identifiers, or has malformed scope metadata';
    END IF;

    IF EXISTS (
        SELECT classified.name
        FROM (
            SELECT
                sv.name,
                COALESCE(NULLIF(sv.envelope_json #>> '{scope,scope}', ''),
                    CASE WHEN NULLIF(sv.envelope_json #>> '{scope,fleet}', '') IS NOT NULL THEN 'fleet' ELSE 'endpoint' END) AS scope_type,
                COALESCE(NULLIF(sv.envelope_json #>> '{scope,fleet}', ''), NULLIF(sv.envelope_json #>> '{scope,endpointId}', '')) AS scope_id
            FROM secret_versions sv
        ) classified
        GROUP BY classified.name
        HAVING count(DISTINCT classified.scope_type || E'\x00' || COALESCE(classified.scope_id, '')) <> 1
    ) THEN
        RAISE EXCEPTION 'logical secret versions disagree on scope';
    END IF;

    IF EXISTS (
        SELECT 1 FROM secret_names sn
        WHERE NOT EXISTS (SELECT 1 FROM secret_versions sv WHERE sv.name = sn.name)
    ) THEN
        RAISE EXCEPTION 'legacy secret name has no version scope evidence';
    END IF;
END
$$;
WITH classified AS (
    SELECT DISTINCT ON (sv.name)
        sv.name,
        COALESCE(NULLIF(sv.envelope_json #>> '{scope,scope}', ''),
            CASE WHEN NULLIF(sv.envelope_json #>> '{scope,fleet}', '') IS NOT NULL THEN 'fleet' ELSE 'endpoint' END) AS scope_type,
        COALESCE(NULLIF(sv.envelope_json #>> '{scope,fleet}', ''), NULLIF(sv.envelope_json #>> '{scope,endpointId}', '')) AS scope_id
    FROM secret_versions sv
    ORDER BY sv.name, sv.version
)
UPDATE secret_names sn
SET scope_type = classified.scope_type,
    scope_id = classified.scope_id
FROM classified
WHERE sn.name = classified.name
  AND (sn.scope_type IS NULL OR (sn.scope_type = classified.scope_type AND sn.scope_id IS NOT DISTINCT FROM classified.scope_id));

ALTER TABLE secret_names
    ALTER COLUMN scope_type SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'secret_names_scope_type') THEN
        ALTER TABLE secret_names ADD CONSTRAINT secret_names_scope_type
            CHECK (scope_type IN ('global', 'fleet', 'endpoint'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'secret_names_scope_identifier') THEN
        ALTER TABLE secret_names ADD CONSTRAINT secret_names_scope_identifier CHECK (
            (scope_type = 'global' AND scope_id IS NULL)
            OR (scope_type IN ('fleet', 'endpoint') AND scope_id IS NOT NULL AND scope_id <> '' AND scope_id = btrim(scope_id))
        );
    END IF;
END
$$;
