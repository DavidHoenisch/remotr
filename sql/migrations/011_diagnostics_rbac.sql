INSERT INTO rbac_roles (name, description, built_in) VALUES
    ('diagnostics_collector', 'Request and download endpoint diagnostic bundles.', true)
ON CONFLICT (name) DO NOTHING;

INSERT INTO rbac_rules (id, role_name, method, path_pattern)
SELECT gen_random_uuid(), 'diagnostics_collector', method, path_pattern
FROM (VALUES
    ('POST', '/v1/admin/endpoints/*/diagnostics/collect'),
    ('GET', '/v1/admin/diagnostics/*')
) AS rules(method, path_pattern)
WHERE NOT EXISTS (
    SELECT 1 FROM rbac_rules
    WHERE role_name = 'diagnostics_collector'
      AND method = rules.method
      AND path_pattern = rules.path_pattern
);
