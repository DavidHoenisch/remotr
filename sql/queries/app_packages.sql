-- name: CreateAppPackage :one
INSERT INTO app_packages (id, name, version, s3_key, sha256, manifest)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, name, version, s3_key, sha256, manifest, created_at;

-- name: GetAppPackage :one
SELECT id, name, version, s3_key, sha256, manifest, created_at
FROM app_packages
WHERE name = $1 AND version = $2;

-- name: ListAppPackages :many
SELECT id, name, version, s3_key, sha256, manifest, created_at
FROM app_packages
WHERE ($1::text = '' OR name LIKE $1 || '%')
ORDER BY name, version;

-- name: DeleteAppPackage :execrows
DELETE FROM app_packages
WHERE name = $1 AND version = $2;
