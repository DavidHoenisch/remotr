-- name: UpsertEndpointLabel :exec
INSERT INTO endpoint_labels (endpoint_id, key, value, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (endpoint_id, key) DO UPDATE
    SET value = EXCLUDED.value,
        updated_at = now();

-- name: ListEndpointLabels :many
SELECT endpoint_id, key, value
FROM endpoint_labels
ORDER BY endpoint_id, key;

-- name: ListEndpointLabelsForEndpoint :many
SELECT key, value
FROM endpoint_labels
WHERE endpoint_id = $1
ORDER BY key;
