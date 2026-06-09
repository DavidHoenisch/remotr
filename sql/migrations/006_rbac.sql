ALTER TABLE operator_credentials
    ADD COLUMN IF NOT EXISTS operator_id TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS operator_credentials_operator_id_active_idx
    ON operator_credentials (operator_id)
    WHERE operator_id IS NOT NULL AND revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS rbac_roles (
    name TEXT PRIMARY KEY,
    description TEXT NOT NULL DEFAULT '',
    built_in BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS rbac_rules (
    id UUID PRIMARY KEY,
    role_name TEXT NOT NULL REFERENCES rbac_roles (name) ON DELETE CASCADE,
    method TEXT NOT NULL,
    path_pattern TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (role_name, method, path_pattern)
);

CREATE TABLE IF NOT EXISTS operator_role_assignments (
    operator_id TEXT NOT NULL,
    role_name TEXT NOT NULL REFERENCES rbac_roles (name) ON DELETE CASCADE,
    PRIMARY KEY (operator_id, role_name)
);

INSERT INTO rbac_roles (name, description, built_in) VALUES
    ('global_admin', 'Full administrative access to all operator API routes.', true),
    ('read_only', 'Read-only access to all operator API routes.', true),
    ('security_logger', 'Read audit events and use the SIEM export endpoint.', true)
ON CONFLICT (name) DO NOTHING;
