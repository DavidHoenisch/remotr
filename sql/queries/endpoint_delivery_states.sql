-- name: UpsertEndpointDeliveryState :one
INSERT INTO endpoint_delivery_states (
    endpoint_id, target_release_ref,
    offered_release_ref, offered_digest, offered_schema_version, offered_at,
    active_release_ref, active_digest, active_schema_version, active_at,
    capability_blocked_target_ref, missing_requirements, unmanaged, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW()
)
ON CONFLICT (endpoint_id) DO UPDATE SET
    target_release_ref = EXCLUDED.target_release_ref,
    offered_release_ref = EXCLUDED.offered_release_ref,
    offered_digest = EXCLUDED.offered_digest,
    offered_schema_version = EXCLUDED.offered_schema_version,
    offered_at = EXCLUDED.offered_at,
    active_release_ref = EXCLUDED.active_release_ref,
    active_digest = EXCLUDED.active_digest,
    active_schema_version = EXCLUDED.active_schema_version,
    active_at = EXCLUDED.active_at,
    capability_blocked_target_ref = EXCLUDED.capability_blocked_target_ref,
    missing_requirements = EXCLUDED.missing_requirements,
    unmanaged = EXCLUDED.unmanaged,
    updated_at = NOW()
RETURNING *;

-- name: GetEndpointDeliveryState :one
SELECT * FROM endpoint_delivery_states WHERE endpoint_id = $1;
