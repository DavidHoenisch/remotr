package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/DavidHoenisch/remotr/internal/queryscheduler"
)

// QueryExecutionRecord is one scheduled or ad-hoc osquery run stored in Postgres.
type QueryExecutionRecord struct {
	RunID        string     `json:"run_id"`
	EndpointID   string     `json:"endpoint_id"`
	Fleet        string     `json:"fleet"`
	QueryName    string     `json:"query_name"`
	TriggerType  string     `json:"trigger_type"`
	Status       string     `json:"status"`
	RowCount     int        `json:"row_count"`
	Truncated    bool       `json:"truncated"`
	DurationMS   int64      `json:"duration_ms"`
	ErrorSummary string     `json:"error_summary,omitempty"`
	ScheduledFor *time.Time `json:"scheduled_for,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

// QuerySnapshotRecord is the latest snapshot of rows for one (endpoint, query).
type QuerySnapshotRecord struct {
	EndpointID string              `json:"endpoint_id"`
	QueryName  string              `json:"query_name"`
	Fleet      string              `json:"fleet"`
	RunID      string              `json:"run_id"`
	Rows       []map[string]string `json:"rows"`
	RowCount   int                 `json:"row_count"`
	Truncated  bool                `json:"truncated"`
	UpdatedAt  time.Time           `json:"updated_at"`
}

// QueryDiffRecord stores added and removed rows for one mode: diff execution.
type QueryDiffRecord struct {
	RunID        string              `json:"run_id"`
	EndpointID   string              `json:"endpoint_id"`
	Fleet        string              `json:"fleet"`
	QueryName    string              `json:"query_name"`
	AddedRows    []map[string]string `json:"added_rows"`
	RemovedRows  []map[string]string `json:"removed_rows"`
	AddedCount   int                 `json:"added_count"`
	RemovedCount int                 `json:"removed_count"`
	CreatedAt    time.Time           `json:"created_at"`
}

// AdhocQueryRequestRecord tracks one operator-initiated live query request.
type AdhocQueryRequestRecord struct {
	RequestID  string    `json:"request_id"`
	Actor      string    `json:"actor"`
	Fleet      string    `json:"fleet,omitempty"`
	EndpointID string    `json:"endpoint_id,omitempty"`
	QueryName  string    `json:"query_name,omitempty"`
	SQL        string    `json:"sql"`
	IsInline   bool      `json:"is_inline"`
	TimeoutSec int       `json:"timeout_sec"`
	MaxRows    int       `json:"max_rows"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// AdhocQueryResultRecord stores one endpoint's response to an ad-hoc query request.
type AdhocQueryResultRecord struct {
	RequestID    string              `json:"request_id"`
	EndpointID   string              `json:"endpoint_id"`
	Status       string              `json:"status"`
	Rows         []map[string]string `json:"rows"`
	RowCount     int                 `json:"row_count"`
	Truncated    bool                `json:"truncated"`
	DurationMS   int64               `json:"duration_ms"`
	ErrorSummary string              `json:"error_summary,omitempty"`
	CompletedAt  time.Time           `json:"completed_at"`
}

// QueryResultInput is one query execution result reported by an agent.
type QueryResultInput struct {
	RunID         string
	EndpointID    string
	Fleet         string
	QueryName     string
	TriggerType   string
	Mode          string // snapshot | diff
	RetentionRuns int
	Status        string
	Rows          []map[string]string
	RowCount      int
	Truncated     bool
	DurationMS    int64
	ErrorSummary  string
	ScheduledFor  *time.Time
	CompletedAt   time.Time
}

var ErrAdhocQueryNotFound = errors.New("ad-hoc query request not found")

func (s *Store) requirePool() error {
	if s == nil || s.pool == nil {
		return errors.New("postgres pool not configured")
	}
	return nil
}

// MarkQueryRunning records an in-flight scheduled query dispatch.
func (s *Store) MarkQueryRunning(ctx context.Context, endpointID, fleet string, due queryscheduler.DueQuery, startedAt time.Time) error {
	if err := s.requirePool(); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
INSERT INTO query_executions (
    run_id, endpoint_id, fleet, query_name, trigger_type, status,
    row_count, truncated, duration_ms, error_summary, scheduled_for, started_at, completed_at, updated_at
) VALUES ($1, $2, $3, $4, 'scheduled', 'running', 0, FALSE, 0, '', $5, $6, NULL, $6)
ON CONFLICT (run_id) DO NOTHING`,
		due.RunID, endpointID, fleet, due.Query.Name, due.ScheduledFor.UTC(), startedAt.UTC(),
	)
	return err
}

// ListQueryLastRuns returns the latest scheduled execution per query for an endpoint.
func (s *Store) ListQueryLastRuns(ctx context.Context, endpointID string) (map[string]queryscheduler.LastRun, error) {
	if err := s.requirePool(); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT DISTINCT ON (query_name)
    query_name, run_id, scheduled_for, status, started_at, completed_at, updated_at
FROM query_executions
WHERE endpoint_id = $1 AND trigger_type = 'scheduled'
ORDER BY query_name, started_at DESC`, endpointID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]queryscheduler.LastRun{}
	for rows.Next() {
		var r queryscheduler.LastRun
		var sched, completed *time.Time
		if err := rows.Scan(&r.QueryName, &r.RunID, &sched, &r.Status, &r.StartedAt, &completed, &r.UpdatedAt); err != nil {
			return nil, err
		}
		if sched != nil {
			r.ScheduledFor = sched.UTC()
		}
		if completed != nil {
			r.CompletedAt = completed.UTC()
		}
		out[r.QueryName] = r
	}
	return out, rows.Err()
}

// RecordQueryResult persists an execution, updates the latest snapshot, and for
// mode=diff computes and stores the canonical row diff with bounded retention.
func (s *Store) RecordQueryResult(ctx context.Context, in QueryResultInput) error {
	if err := s.requirePool(); err != nil {
		return err
	}
	completedAt := in.CompletedAt.UTC()
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	}
	trigger := in.TriggerType
	if trigger == "" {
		trigger = "scheduled"
	}

	_, err := s.pool.Exec(ctx, `
INSERT INTO query_executions (
    run_id, endpoint_id, fleet, query_name, trigger_type, status,
    row_count, truncated, duration_ms, error_summary, scheduled_for, started_at, completed_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $12, $12)
ON CONFLICT (run_id) DO UPDATE SET
    status        = EXCLUDED.status,
    row_count     = EXCLUDED.row_count,
    truncated     = EXCLUDED.truncated,
    duration_ms   = EXCLUDED.duration_ms,
    error_summary = EXCLUDED.error_summary,
    completed_at  = EXCLUDED.completed_at,
    updated_at    = EXCLUDED.updated_at`,
		in.RunID, in.EndpointID, in.Fleet, in.QueryName, trigger, in.Status,
		in.RowCount, in.Truncated, in.DurationMS, in.ErrorSummary, in.ScheduledFor, completedAt,
	)
	if err != nil {
		return err
	}

	if in.Status != "success" {
		return nil
	}

	if in.Mode == "diff" {
		var prevRaw []byte
		err := s.pool.QueryRow(ctx, `
SELECT rows_json FROM query_snapshots
WHERE endpoint_id = $1 AND query_name = $2`,
			in.EndpointID, in.QueryName,
		).Scan(&prevRaw)
		var prevRows []map[string]string
		if err == nil && len(prevRaw) > 0 {
			_ = json.Unmarshal(prevRaw, &prevRows)
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		diff := queryscheduler.ComputeRowDiff(prevRows, in.Rows)
		addedJSON, _ := json.Marshal(diff.Added)
		removedJSON, _ := json.Marshal(diff.Removed)
		if _, err := s.pool.Exec(ctx, `
INSERT INTO query_diffs (run_id, endpoint_id, fleet, query_name, added_rows, removed_rows, added_count, removed_count, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (run_id) DO NOTHING`,
			in.RunID, in.EndpointID, in.Fleet, in.QueryName, addedJSON, removedJSON, len(diff.Added), len(diff.Removed), completedAt,
		); err != nil {
			return err
		}
		retention := in.RetentionRuns
		if retention <= 0 {
			retention = 20
		}
		if _, err := s.pool.Exec(ctx, `
DELETE FROM query_diffs
WHERE run_id IN (
    SELECT d.run_id FROM query_diffs d
    WHERE d.endpoint_id = $1 AND d.query_name = $2
    ORDER BY d.created_at DESC
    OFFSET $3
)`, in.EndpointID, in.QueryName, retention); err != nil {
			return err
		}
	}

	rowsJSON, err := json.Marshal(nonNilRows(in.Rows))
	if err != nil {
		return fmt.Errorf("marshal snapshot rows: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO query_snapshots (endpoint_id, query_name, fleet, run_id, rows_json, row_count, truncated, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (endpoint_id, query_name) DO UPDATE SET
    fleet      = EXCLUDED.fleet,
    run_id     = EXCLUDED.run_id,
    rows_json  = EXCLUDED.rows_json,
    row_count  = EXCLUDED.row_count,
    truncated  = EXCLUDED.truncated,
    updated_at = EXCLUDED.updated_at`,
		in.EndpointID, in.QueryName, in.Fleet, in.RunID, rowsJSON, in.RowCount, in.Truncated, completedAt,
	)
	return err
}

// ListQueryStatuses returns the latest execution per endpoint for a query.
func (s *Store) ListQueryStatuses(ctx context.Context, fleet, queryName string) ([]QueryExecutionRecord, error) {
	if err := s.requirePool(); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT DISTINCT ON (endpoint_id)
    run_id, endpoint_id, fleet, query_name, trigger_type, status,
    row_count, truncated, duration_ms, error_summary, scheduled_for, started_at, completed_at
FROM query_executions
WHERE ($1::text = '' OR fleet = $1) AND query_name = $2
ORDER BY endpoint_id, started_at DESC`, fleet, queryName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []QueryExecutionRecord
	for rows.Next() {
		var r QueryExecutionRecord
		if err := rows.Scan(&r.RunID, &r.EndpointID, &r.Fleet, &r.QueryName, &r.TriggerType, &r.Status,
			&r.RowCount, &r.Truncated, &r.DurationMS, &r.ErrorSummary, &r.ScheduledFor, &r.StartedAt, &r.CompletedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListQuerySnapshots returns latest snapshots filtered by queryName, optional fleet, and optional endpointID.
func (s *Store) ListQuerySnapshots(ctx context.Context, queryName, fleet, endpointID string) ([]QuerySnapshotRecord, error) {
	if err := s.requirePool(); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT endpoint_id, query_name, fleet, run_id, rows_json, row_count, truncated, updated_at
FROM query_snapshots
WHERE query_name = $1
  AND ($2::text = '' OR fleet = $2)
  AND ($3::text = '' OR endpoint_id = $3)
ORDER BY endpoint_id`, queryName, fleet, endpointID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []QuerySnapshotRecord
	for rows.Next() {
		var r QuerySnapshotRecord
		var raw []byte
		if err := rows.Scan(&r.EndpointID, &r.QueryName, &r.Fleet, &r.RunID, &raw, &r.RowCount, &r.Truncated, &r.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &r.Rows)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListQueryDiffs returns recent diff records for a query.
func (s *Store) ListQueryDiffs(ctx context.Context, queryName, fleet, endpointID string, since time.Time, limit int) ([]QueryDiffRecord, error) {
	if err := s.requirePool(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
SELECT run_id, endpoint_id, fleet, query_name, added_rows, removed_rows, added_count, removed_count, created_at
FROM query_diffs
WHERE query_name = $1
  AND ($2::text = '' OR fleet = $2)
  AND ($3::text = '' OR endpoint_id = $3)
  AND created_at >= $4
ORDER BY created_at DESC
LIMIT $5`, queryName, fleet, endpointID, since.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []QueryDiffRecord
	for rows.Next() {
		var r QueryDiffRecord
		var addedRaw, removedRaw []byte
		if err := rows.Scan(&r.RunID, &r.EndpointID, &r.Fleet, &r.QueryName, &addedRaw, &removedRaw, &r.AddedCount, &r.RemovedCount, &r.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(addedRaw, &r.AddedRows)
		_ = json.Unmarshal(removedRaw, &r.RemovedRows)
		out = append(out, r)
	}
	return out, rows.Err()
}

// CreateAdhocQueryRequest inserts a new TTL-bounded ad-hoc live query request.
func (s *Store) CreateAdhocQueryRequest(ctx context.Context, req AdhocQueryRequestRecord) error {
	if err := s.requirePool(); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
INSERT INTO query_adhoc_requests (
    request_id, actor, fleet, endpoint_id, query_name, sql, is_inline, timeout_sec, max_rows, created_at, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		req.RequestID, req.Actor, req.Fleet, req.EndpointID, req.QueryName, req.SQL,
		req.IsInline, req.TimeoutSec, req.MaxRows, req.CreatedAt.UTC(), req.ExpiresAt.UTC(),
	)
	return err
}

// GetAdhocQueryRequest loads one ad-hoc query request by ID.
func (s *Store) GetAdhocQueryRequest(ctx context.Context, requestID string) (AdhocQueryRequestRecord, error) {
	if err := s.requirePool(); err != nil {
		return AdhocQueryRequestRecord{}, err
	}
	var r AdhocQueryRequestRecord
	err := s.pool.QueryRow(ctx, `
SELECT request_id, actor, fleet, endpoint_id, query_name, sql, is_inline, timeout_sec, max_rows, created_at, expires_at
FROM query_adhoc_requests
WHERE request_id = $1`, requestID).Scan(
		&r.RequestID, &r.Actor, &r.Fleet, &r.EndpointID, &r.QueryName, &r.SQL,
		&r.IsInline, &r.TimeoutSec, &r.MaxRows, &r.CreatedAt, &r.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdhocQueryRequestRecord{}, ErrAdhocQueryNotFound
	}
	return r, err
}

// ListPendingAdhocQueryRequests returns non-expired ad-hoc requests not yet reported by endpointID.
func (s *Store) ListPendingAdhocQueryRequests(ctx context.Context, now time.Time, endpointID, fleet string) ([]AdhocQueryRequestRecord, error) {
	if err := s.requirePool(); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT r.request_id, r.actor, r.fleet, r.endpoint_id, r.query_name, r.sql, r.is_inline, r.timeout_sec, r.max_rows, r.created_at, r.expires_at
FROM query_adhoc_requests r
WHERE r.expires_at > $1
  AND (r.endpoint_id = $2 OR (r.endpoint_id = '' AND r.fleet = $3))
  AND NOT EXISTS (
      SELECT 1 FROM query_adhoc_results res
      WHERE res.request_id = r.request_id AND res.endpoint_id = $2
  )
ORDER BY r.created_at ASC
LIMIT 16`, now.UTC(), endpointID, fleet)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AdhocQueryRequestRecord
	for rows.Next() {
		var r AdhocQueryRequestRecord
		if err := rows.Scan(&r.RequestID, &r.Actor, &r.Fleet, &r.EndpointID, &r.QueryName, &r.SQL,
			&r.IsInline, &r.TimeoutSec, &r.MaxRows, &r.CreatedAt, &r.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RecordAdhocQueryResult persists one endpoint's ad-hoc live query response.
func (s *Store) RecordAdhocQueryResult(ctx context.Context, res AdhocQueryResultRecord) error {
	if err := s.requirePool(); err != nil {
		return err
	}
	rowsJSON, err := json.Marshal(nonNilRows(res.Rows))
	if err != nil {
		return err
	}
	completed := res.CompletedAt.UTC()
	if completed.IsZero() {
		completed = time.Now().UTC()
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO query_adhoc_results (
    request_id, endpoint_id, status, rows_json, row_count, truncated, duration_ms, error_summary, completed_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (request_id, endpoint_id) DO UPDATE SET
    status        = EXCLUDED.status,
    rows_json     = EXCLUDED.rows_json,
    row_count     = EXCLUDED.row_count,
    truncated     = EXCLUDED.truncated,
    duration_ms   = EXCLUDED.duration_ms,
    error_summary = EXCLUDED.error_summary,
    completed_at  = EXCLUDED.completed_at`,
		res.RequestID, res.EndpointID, res.Status, rowsJSON, res.RowCount, res.Truncated, res.DurationMS, res.ErrorSummary, completed,
	)
	return err
}

// ListAdhocQueryResults returns all endpoint results recorded for an ad-hoc request.
func (s *Store) ListAdhocQueryResults(ctx context.Context, requestID string) ([]AdhocQueryResultRecord, error) {
	if err := s.requirePool(); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT request_id, endpoint_id, status, rows_json, row_count, truncated, duration_ms, error_summary, completed_at
FROM query_adhoc_results
WHERE request_id = $1
ORDER BY endpoint_id`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AdhocQueryResultRecord
	for rows.Next() {
		var r AdhocQueryResultRecord
		var raw []byte
		if err := rows.Scan(&r.RequestID, &r.EndpointID, &r.Status, &raw, &r.RowCount, &r.Truncated, &r.DurationMS, &r.ErrorSummary, &r.CompletedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &r.Rows)
		out = append(out, r)
	}
	return out, rows.Err()
}

// PruneExpiredAdhocQueries removes ad-hoc requests past cutoff.
func (s *Store) PruneExpiredAdhocQueries(ctx context.Context, cutoff time.Time) error {
	if err := s.requirePool(); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM query_adhoc_requests WHERE expires_at < $1`, cutoff.UTC())
	return err
}

func nonNilRows(rows []map[string]string) []map[string]string {
	if rows == nil {
		return []map[string]string{}
	}
	return rows
}
