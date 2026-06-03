-- name: SetEndpointDesiredAgentVersion :one
UPDATE endpoints
SET desired_agent_version = $2,
    desired_agent_version_at = now(),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetFleetDesiredAgentVersion :execrows
UPDATE endpoints
SET desired_agent_version = $2,
    desired_agent_version_at = now(),
    updated_at = now()
WHERE fleet = $1;

-- name: ClearEndpointDesiredAgentVersion :one
UPDATE endpoints
SET desired_agent_version = NULL,
    desired_agent_version_at = NULL,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateEndpointAgentUpgradeReport :one
UPDATE endpoints
SET reported_agent_version = $2,
    agent_upgrade_phase = $3,
    agent_upgrade_message = $4,
    agent_upgrade_reported_at = now(),
    desired_agent_version = CASE
        WHEN sqlc.arg(clear_desired)::boolean THEN NULL
        ELSE desired_agent_version
    END,
    desired_agent_version_at = CASE
        WHEN sqlc.arg(clear_desired)::boolean THEN NULL
        ELSE desired_agent_version_at
    END,
    updated_at = now()
WHERE id = $1
RETURNING *;
