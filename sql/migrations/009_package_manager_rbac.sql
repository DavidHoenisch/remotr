INSERT INTO rbac_roles (name, description, built_in) VALUES
    ('package_manager', 'Manage custom Remotr app packages in the catalog.', true)
ON CONFLICT (name) DO NOTHING;
