-- name: UpsertRBACRole :exec
INSERT INTO rbac_roles (name, description, built_in)
VALUES ($1, $2, $3)
ON CONFLICT (name) DO UPDATE
    SET description = EXCLUDED.description,
        built_in = EXCLUDED.built_in;

-- name: ListRBACRoles :many
SELECT name, description, built_in, created_at
FROM rbac_roles
ORDER BY built_in DESC, name;

-- name: GetRBACRole :one
SELECT name, description, built_in, created_at
FROM rbac_roles
WHERE name = $1;

-- name: DeleteRBACRole :execrows
DELETE FROM rbac_roles
WHERE name = $1 AND built_in = false;

-- name: ListRBACRulesForRole :many
SELECT id, role_name, method, path_pattern, created_at
FROM rbac_rules
WHERE role_name = $1
ORDER BY created_at, id;

-- name: InsertRBACRule :one
INSERT INTO rbac_rules (id, role_name, method, path_pattern)
VALUES ($1, $2, $3, $4)
RETURNING id, role_name, method, path_pattern, created_at;

-- name: DeleteRBACRule :execrows
DELETE FROM rbac_rules
WHERE id = $1 AND role_name = $2;

-- name: ListOperatorRoleAssignments :many
SELECT operator_id, role_name
FROM operator_role_assignments
ORDER BY operator_id, role_name;

-- name: ListOperatorRoleAssignmentsForOperator :many
SELECT role_name
FROM operator_role_assignments
WHERE operator_id = $1
ORDER BY role_name;

-- name: ReplaceOperatorRoleAssignments :exec
DELETE FROM operator_role_assignments
WHERE operator_id = $1;

-- name: InsertOperatorRoleAssignment :exec
INSERT INTO operator_role_assignments (operator_id, role_name)
VALUES ($1, $2);

-- name: ListActiveOperators :many
SELECT cert_fingerprint, operator_id, created_at, revoked_at
FROM operator_credentials
WHERE revoked_at IS NULL
ORDER BY created_at;
