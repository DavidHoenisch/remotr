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
WHERE endpoint_delivery_states.target_release_ref IS DISTINCT FROM EXCLUDED.target_release_ref
   OR endpoint_delivery_states.offered_release_ref IS DISTINCT FROM EXCLUDED.offered_release_ref
   OR endpoint_delivery_states.offered_digest IS DISTINCT FROM EXCLUDED.offered_digest
   OR endpoint_delivery_states.offered_schema_version IS DISTINCT FROM EXCLUDED.offered_schema_version
   OR endpoint_delivery_states.active_release_ref IS DISTINCT FROM EXCLUDED.active_release_ref
   OR endpoint_delivery_states.active_digest IS DISTINCT FROM EXCLUDED.active_digest
   OR endpoint_delivery_states.active_schema_version IS DISTINCT FROM EXCLUDED.active_schema_version
   OR endpoint_delivery_states.capability_blocked_target_ref IS DISTINCT FROM EXCLUDED.capability_blocked_target_ref
   OR endpoint_delivery_states.missing_requirements IS DISTINCT FROM EXCLUDED.missing_requirements
   OR endpoint_delivery_states.unmanaged IS DISTINCT FROM EXCLUDED.unmanaged
RETURNING *;

-- name: GetEndpointDeliveryState :one
SELECT * FROM endpoint_delivery_states WHERE endpoint_id = $1;
