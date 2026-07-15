-- name: LoadChangeControlState :one
SELECT state_json, revision
FROM change_control_state
WHERE singleton = TRUE;

-- name: SaveChangeControlState :one
WITH updated AS (
    UPDATE change_control_state
    SET state_json = sqlc.arg(state_json),
        revision = revision + 1,
        updated_at = now()
    WHERE singleton = TRUE
      AND revision = sqlc.arg(expected_revision)
    RETURNING revision
), inserted AS (
    INSERT INTO change_control_state (singleton, state_json, revision, updated_at)
    SELECT TRUE, sqlc.arg(state_json), 1, now()
    WHERE sqlc.arg(expected_revision) = 0
      AND NOT EXISTS (SELECT 1 FROM updated)
    ON CONFLICT (singleton) DO NOTHING
    RETURNING revision
)
SELECT revision FROM updated
UNION ALL
SELECT revision FROM inserted;
