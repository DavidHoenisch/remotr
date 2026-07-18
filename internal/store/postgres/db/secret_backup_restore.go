package db

import "context"

const listSecretVersionEnvelopes = `-- name: ListSecretVersionEnvelopes :many
SELECT envelope_json
FROM secret_versions
ORDER BY name, version
`

// ListSecretVersionEnvelopes returns every application-encrypted record for
// startup backup/restore validation. It intentionally lives outside generated
// bindings because the result is a single stable byte column.
func (q *Queries) ListSecretVersionEnvelopes(ctx context.Context) ([][]byte, error) {
	rows, err := q.db.Query(ctx, listSecretVersionEnvelopes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([][]byte, 0)
	for rows.Next() {
		var envelope []byte
		if err := rows.Scan(&envelope); err != nil {
			return nil, err
		}
		records = append(records, append([]byte(nil), envelope...))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}
