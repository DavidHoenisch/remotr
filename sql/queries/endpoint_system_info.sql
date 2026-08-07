-- name: UpsertEndpointSystemInfo :one
INSERT INTO endpoint_system_info (endpoint_id, digest, info_json, reported_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (endpoint_id) DO UPDATE
SET digest = EXCLUDED.digest,
    info_json = EXCLUDED.info_json,
    reported_at = now()
WHERE endpoint_system_info.digest IS DISTINCT FROM EXCLUDED.digest
   OR endpoint_system_info.info_json IS DISTINCT FROM EXCLUDED.info_json
RETURNING *;

-- name: GetEndpointSystemInfo :one
SELECT endpoint_id, digest, info_json, reported_at
FROM endpoint_system_info
WHERE endpoint_id = $1;
