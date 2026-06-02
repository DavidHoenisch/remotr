-- name: EnsureFleet :exec
INSERT INTO fleet_settings (fleet, remediation_policy)
VALUES ($1, 'auto')
ON CONFLICT (fleet) DO NOTHING;

-- name: UpsertFleetSettings :one
INSERT INTO fleet_settings (fleet, remediation_policy)
VALUES ($1, $2)
ON CONFLICT (fleet) DO UPDATE
    SET remediation_policy = EXCLUDED.remediation_policy,
        updated_at = now()
RETURNING *;

-- name: GetFleetSettings :one
SELECT * FROM fleet_settings
WHERE fleet = $1;
