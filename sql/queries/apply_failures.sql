-- name: InsertApplyFailure :exec
INSERT INTO apply_failures (id, endpoint_id, release_ref, resource_address, message, reported_at)
VALUES ($1, $2, $3, $4, $5, now());
