-- name: InsertFirewallAuditReport :exec
INSERT INTO firewall_audit_reports (id, endpoint_id, digest, report_json, reported_at)
VALUES (gen_random_uuid(), $1, $2, $3, now());

-- name: GetLatestFirewallAuditReport :one
SELECT id, endpoint_id, digest, report_json, reported_at
FROM firewall_audit_reports
WHERE endpoint_id = $1
ORDER BY reported_at DESC
LIMIT 1;

-- name: ListFleetFirewallAuditReports :many
SELECT id, endpoint_id, digest, report_json, reported_at
FROM firewall_audit_reports
WHERE endpoint_id IN (SELECT id FROM endpoints WHERE fleet = $1)
ORDER BY reported_at DESC;

-- name: PruneOldFirewallAuditReports :exec
DELETE FROM firewall_audit_reports
WHERE reported_at < $1;
