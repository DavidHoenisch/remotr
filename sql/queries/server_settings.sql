-- name: GetServerSetting :one
SELECT value FROM server_settings
WHERE key = $1;

-- name: UpsertServerSetting :exec
INSERT INTO server_settings (key, value)
VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE
    SET value = EXCLUDED.value;
