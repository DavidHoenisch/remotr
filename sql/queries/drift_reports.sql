-- name: InsertDriftReport :exec
INSERT INTO drift_reports (id, endpoint_id, release_ref, digest, report_json, reported_at)
VALUES ($1, $2, $3, $4, $5, now());

-- name: GetLatestDriftReport :one
SELECT id, endpoint_id, release_ref, digest, report_json, reported_at
FROM drift_reports
WHERE endpoint_id = $1
ORDER BY reported_at DESC
LIMIT 1;
