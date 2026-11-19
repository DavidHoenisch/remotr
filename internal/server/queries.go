package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	stdsync "sync"
	"time"

	"github.com/DavidHoenisch/remotr/internal/configcompose"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/queryscheduler"
	pgstore "github.com/DavidHoenisch/remotr/internal/store/postgres"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// QueryStore persists scheduled and ad-hoc osquery executions, snapshots, and diffs.
type QueryStore interface {
	MarkQueryRunning(ctx context.Context, endpointID, fleet string, due queryscheduler.DueQuery, startedAt time.Time) error
	ListQueryLastRuns(ctx context.Context, endpointID string) (map[string]queryscheduler.LastRun, error)
	RecordQueryResult(ctx context.Context, in pgstore.QueryResultInput) error
	ListQueryStatuses(ctx context.Context, fleet, queryName string) ([]pgstore.QueryExecutionRecord, error)
	ListQuerySnapshots(ctx context.Context, queryName, fleet, endpointID string) ([]pgstore.QuerySnapshotRecord, error)
	ListQueryDiffs(ctx context.Context, queryName, fleet, endpointID string, since time.Time, limit int) ([]pgstore.QueryDiffRecord, error)
	CreateAdhocQueryRequest(ctx context.Context, req pgstore.AdhocQueryRequestRecord) error
	GetAdhocQueryRequest(ctx context.Context, requestID string) (pgstore.AdhocQueryRequestRecord, error)
	ListPendingAdhocQueryRequests(ctx context.Context, now time.Time, endpointID, fleet string) ([]pgstore.AdhocQueryRequestRecord, error)
	RecordAdhocQueryResult(ctx context.Context, res pgstore.AdhocQueryResultRecord) error
	ListAdhocQueryResults(ctx context.Context, requestID string) ([]pgstore.AdhocQueryResultRecord, error)
	PruneExpiredAdhocQueries(ctx context.Context, cutoff time.Time) error
}

type dueQueryPayload struct {
	RunID        string    `json:"runId"`
	RequestID    string    `json:"requestId,omitempty"`
	TriggerType  string    `json:"triggerType"` // scheduled | adhoc
	Name         string    `json:"name"`
	SQL          string    `json:"sql"`
	TimeoutSec   int       `json:"timeoutSec"`
	MaxRows      int       `json:"maxRows"`
	ScheduledFor time.Time `json:"scheduledFor,omitempty"`
}

type queryResultPayload struct {
	RunID        string              `json:"runId"`
	RequestID    string              `json:"requestId,omitempty"`
	TriggerType  string              `json:"triggerType,omitempty"`
	Name         string              `json:"name"`
	Status       string              `json:"status"`
	Rows         []map[string]string `json:"rows,omitempty"`
	RowCount     int                 `json:"rowCount"`
	Truncated    bool                `json:"truncated,omitempty"`
	DurationMS   int64               `json:"durationMs"`
	ErrorSummary string              `json:"errorSummary,omitempty"`
	ScheduledFor *time.Time          `json:"scheduledFor,omitempty"`
	CompletedAt  time.Time           `json:"completedAt,omitempty"`
}

func resolveQueriesArtifact(ctx context.Context, store ArtifactReader, repoRoot, fleet, endpointID, releaseRef string) ([]byte, string, bool, error) {
	if store != nil {
		if endpointID != "" {
			raw, digest, err := store.GetCompiledArtifactForEndpoint(ctx, endpointID, releaseRef, "queries")
			if err == nil {
				return raw, digest, true, nil
			}
			if !errors.Is(err, pgstore.ErrCompiledArtifactNotFound) {
				return nil, "", false, err
			}
		}
		if fleet != "" {
			raw, digest, err := store.GetCompiledArtifactForFleet(ctx, fleet, releaseRef, "queries")
			if err == nil {
				return raw, digest, true, nil
			}
			if !errors.Is(err, pgstore.ErrCompiledArtifactNotFound) {
				return nil, "", false, err
			}
		}
		return nil, "", false, nil
	}
	if strings.TrimSpace(repoRoot) == "" || fleet == "" {
		return nil, "", false, nil
	}
	raw, digest, err := configcompose.RenderFleetQueries(repoRoot, fleet)
	if err != nil {
		return nil, "", false, err
	}
	if len(raw) == 0 {
		return nil, "", false, nil
	}
	return raw, digest, true, nil
}

func (s *Server) loadResolvedQueries(ctx context.Context, fleet, endpointID string) ([]models.QuerySpec, string, bool, error) {
	releaseRef := s.releaseRef(ctx)
	raw, digest, ok, err := resolveQueriesArtifact(ctx, s.cfg.ArtifactStore, s.cfg.ConfigRepoPath, fleet, endpointID, releaseRef)
	if err != nil || !ok {
		return nil, "", ok, err
	}
	state, err := models.ParseQueryState(bytes.NewReader(raw))
	if err != nil {
		return nil, digest, true, err
	}
	return state.Queries, digest, true, nil
}

func (s *Server) dueQueriesForEndpoint(ctx context.Context, endpointID, fleet string, labels map[string]string) []dueQueryPayload {
	if s.cfg.QueryStore == nil {
		return nil
	}
	specs, _, ok, err := s.loadResolvedQueries(ctx, fleet, endpointID)
	if err != nil {
		slog.Warn("load queries artifact", "endpoint", endpointID, "err", err)
		return nil
	}
	if !ok || len(specs) == 0 {
		return nil
	}
	lastRuns, err := s.cfg.QueryStore.ListQueryLastRuns(ctx, endpointID)
	if err != nil {
		slog.Warn("list query last runs", "endpoint", endpointID, "err", err)
		return nil
	}
	now := s.cfg.Now().UTC()
	due := queryscheduler.ListDue(now, specs, lastRuns, types.Distro(labels["distro"]), types.Architecture(labels["arch"]))
	out := make([]dueQueryPayload, 0, len(due))
	for _, d := range due {
		if err := s.cfg.QueryStore.MarkQueryRunning(ctx, endpointID, fleet, d, now); err != nil {
			slog.Warn("mark query running", "endpoint", endpointID, "query", d.Query.Name, "err", err)
			continue
		}
		out = append(out, dueQueryPayload{
			RunID:        d.RunID,
			TriggerType:  "scheduled",
			Name:         d.Query.Name,
			SQL:          d.Query.SQL,
			TimeoutSec:   int(d.Query.EffectiveTimeout().Seconds()),
			MaxRows:      d.Query.EffectiveMaxRows(),
			ScheduledFor: d.ScheduledFor,
		})
	}
	return out
}

func (s *Server) pendingAdhocQueriesForEndpoint(ctx context.Context, endpointID, fleet string) []dueQueryPayload {
	if s.cfg.QueryStore == nil {
		return nil
	}
	now := s.cfg.Now().UTC()
	pending, err := s.cfg.QueryStore.ListPendingAdhocQueryRequests(ctx, now, endpointID, fleet)
	if err != nil {
		slog.Warn("list pending adhoc queries", "endpoint", endpointID, "err", err)
		return nil
	}
	out := make([]dueQueryPayload, 0, len(pending))
	for _, req := range pending {
		name := req.QueryName
		if name == "" {
			name = "adhoc"
		}
		out = append(out, dueQueryPayload{
			RunID:       req.RequestID + ":" + endpointID,
			RequestID:   req.RequestID,
			TriggerType: "adhoc",
			Name:        name,
			SQL:         req.SQL,
			TimeoutSec:  req.TimeoutSec,
			MaxRows:     req.MaxRows,
		})
	}
	return out
}

func (s *Server) persistQueryResults(ctx context.Context, endpointID, fleet string, results []queryResultPayload) {
	if s.cfg.QueryStore == nil || len(results) == 0 {
		return
	}
	specsByName := map[string]models.QuerySpec{}
	if specs, _, ok, err := s.loadResolvedQueries(ctx, fleet, endpointID); err == nil && ok {
		for _, q := range specs {
			specsByName[q.Name] = q
		}
	}
	for _, item := range results {
		completed := item.CompletedAt.UTC()
		if completed.IsZero() {
			completed = s.cfg.Now().UTC()
		}
		if item.RequestID != "" || item.TriggerType == "adhoc" {
			if err := s.cfg.QueryStore.RecordAdhocQueryResult(ctx, pgstore.AdhocQueryResultRecord{
				RequestID:    item.RequestID,
				EndpointID:   endpointID,
				Status:       normalizeQueryStatus(item.Status),
				Rows:         item.Rows,
				RowCount:     item.RowCount,
				Truncated:    item.Truncated,
				DurationMS:   item.DurationMS,
				ErrorSummary: item.ErrorSummary,
				CompletedAt:  completed,
			}); err != nil {
				slog.Warn("record adhoc query result", "endpoint", endpointID, "request", item.RequestID, "err", err)
			}
			continue
		}
		mode := string(models.QueryModeSnapshot)
		retention := models.DefaultQueryRetentionRuns
		if spec, ok := specsByName[item.Name]; ok {
			mode = string(spec.EffectiveMode())
			retention = spec.EffectiveRetentionRuns()
		}
		if err := s.cfg.QueryStore.RecordQueryResult(ctx, pgstore.QueryResultInput{
			RunID:         item.RunID,
			EndpointID:    endpointID,
			Fleet:         fleet,
			QueryName:     item.Name,
			TriggerType:   "scheduled",
			Mode:          mode,
			RetentionRuns: retention,
			Status:        normalizeQueryStatus(item.Status),
			Rows:          item.Rows,
			RowCount:      item.RowCount,
			Truncated:     item.Truncated,
			DurationMS:    item.DurationMS,
			ErrorSummary:  item.ErrorSummary,
			ScheduledFor:  item.ScheduledFor,
			CompletedAt:   completed,
		}); err != nil {
			slog.Warn("record scheduled query result", "endpoint", endpointID, "query", item.Name, "err", err)
		}
	}
}

func normalizeQueryStatus(status string) string {
	switch status {
	case "success", "failed", "unsupported", "running":
		return status
	default:
		return "failed"
	}
}

func (s *Server) handlePostSyncQueryResults(w http.ResponseWriter, r *http.Request) {
	endpointID, err := endpointIDFromRequest(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ep, exists := s.cfg.Registry.EndpointByID(endpointID)
	if !exists || ep.Reserved() {
		http.Error(w, "unknown endpoint", http.StatusForbidden)
		return
	}
	var payload struct {
		Results []queryResultPayload `json:"results"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid query results payload", http.StatusBadRequest)
		return
	}
	s.persistQueryResults(r.Context(), endpointID, ep.Fleet, payload.Results)
	writeJSON(w, map[string]any{"accepted": len(payload.Results)})
}

// MemoryQueryStore is a thread-safe in-memory QueryStore used when Postgres is
// not configured (such as unit and protocol integration tests).
type MemoryQueryStore struct {
	mu           stdsync.RWMutex
	executions   map[string]pgstore.QueryExecutionRecord
	snapshots    map[string]pgstore.QuerySnapshotRecord // key: endpointID + "\x00" + queryName
	diffs        map[string][]pgstore.QueryDiffRecord   // key: endpointID + "\x00" + queryName
	adhocReqs    map[string]pgstore.AdhocQueryRequestRecord
	adhocResults map[string]map[string]pgstore.AdhocQueryResultRecord // requestID -> endpointID -> result
}

func NewMemoryQueryStore() *MemoryQueryStore {
	return &MemoryQueryStore{
		executions:   map[string]pgstore.QueryExecutionRecord{},
		snapshots:    map[string]pgstore.QuerySnapshotRecord{},
		diffs:        map[string][]pgstore.QueryDiffRecord{},
		adhocReqs:    map[string]pgstore.AdhocQueryRequestRecord{},
		adhocResults: map[string]map[string]pgstore.AdhocQueryResultRecord{},
	}
}

func snapKey(endpointID, queryName string) string {
	return endpointID + "\x00" + queryName
}

func (m *MemoryQueryStore) MarkQueryRunning(_ context.Context, endpointID, fleet string, due queryscheduler.DueQuery, startedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sched := due.ScheduledFor.UTC()
	m.executions[due.RunID] = pgstore.QueryExecutionRecord{
		RunID:        due.RunID,
		EndpointID:   endpointID,
		Fleet:        fleet,
		QueryName:    due.Query.Name,
		TriggerType:  "scheduled",
		Status:       "running",
		ScheduledFor: &sched,
		StartedAt:    startedAt.UTC(),
	}
	return nil
}

func (m *MemoryQueryStore) ListQueryLastRuns(_ context.Context, endpointID string) (map[string]queryscheduler.LastRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := map[string]queryscheduler.LastRun{}
	for _, rec := range m.executions {
		if rec.EndpointID != endpointID || rec.TriggerType != "scheduled" {
			continue
		}
		prev, exists := out[rec.QueryName]
		if exists && prev.StartedAt.After(rec.StartedAt) {
			continue
		}
		lr := queryscheduler.LastRun{
			QueryName: rec.QueryName,
			RunID:     rec.RunID,
			Status:    rec.Status,
			StartedAt: rec.StartedAt,
			UpdatedAt: rec.StartedAt,
		}
		if rec.ScheduledFor != nil {
			lr.ScheduledFor = *rec.ScheduledFor
		}
		if rec.CompletedAt != nil {
			lr.CompletedAt = *rec.CompletedAt
		}
		out[rec.QueryName] = lr
	}
	return out, nil
}

func (m *MemoryQueryStore) RecordQueryResult(_ context.Context, in pgstore.QueryResultInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	completed := in.CompletedAt.UTC()
	if completed.IsZero() {
		completed = time.Now().UTC()
	}
	rec := m.executions[in.RunID]
	rec.RunID = in.RunID
	rec.EndpointID = in.EndpointID
	rec.Fleet = in.Fleet
	rec.QueryName = in.QueryName
	rec.TriggerType = "scheduled"
	rec.Status = in.Status
	rec.RowCount = in.RowCount
	rec.Truncated = in.Truncated
	rec.DurationMS = in.DurationMS
	rec.ErrorSummary = in.ErrorSummary
	if in.ScheduledFor != nil {
		s := in.ScheduledFor.UTC()
		rec.ScheduledFor = &s
	}
	if rec.StartedAt.IsZero() {
		rec.StartedAt = completed
	}
	rec.CompletedAt = &completed
	m.executions[in.RunID] = rec

	if in.Status != "success" {
		return nil
	}

	key := snapKey(in.EndpointID, in.QueryName)
	if in.Mode == "diff" {
		prev := m.snapshots[key].Rows
		diff := queryscheduler.ComputeRowDiff(prev, in.Rows)
		m.diffs[key] = append([]pgstore.QueryDiffRecord{{
			RunID:        in.RunID,
			EndpointID:   in.EndpointID,
			Fleet:        in.Fleet,
			QueryName:    in.QueryName,
			AddedRows:    diff.Added,
			RemovedRows:  diff.Removed,
			AddedCount:   len(diff.Added),
			RemovedCount: len(diff.Removed),
			CreatedAt:    completed,
		}}, m.diffs[key]...)
		retention := in.RetentionRuns
		if retention <= 0 {
			retention = models.DefaultQueryRetentionRuns
		}
		if len(m.diffs[key]) > retention {
			m.diffs[key] = m.diffs[key][:retention]
		}
	}

	m.snapshots[key] = pgstore.QuerySnapshotRecord{
		EndpointID: in.EndpointID,
		QueryName:  in.QueryName,
		Fleet:      in.Fleet,
		RunID:      in.RunID,
		Rows:       in.Rows,
		RowCount:   in.RowCount,
		Truncated:  in.Truncated,
		UpdatedAt:  completed,
	}
	return nil
}

func (m *MemoryQueryStore) ListQueryStatuses(_ context.Context, fleet, queryName string) ([]pgstore.QueryExecutionRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	latest := map[string]pgstore.QueryExecutionRecord{}
	for _, rec := range m.executions {
		if rec.QueryName != queryName {
			continue
		}
		if fleet != "" && rec.Fleet != fleet {
			continue
		}
		if prev, ok := latest[rec.EndpointID]; ok && prev.StartedAt.After(rec.StartedAt) {
			continue
		}
		latest[rec.EndpointID] = rec
	}
	out := make([]pgstore.QueryExecutionRecord, 0, len(latest))
	for _, rec := range latest {
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EndpointID < out[j].EndpointID })
	return out, nil
}

func (m *MemoryQueryStore) ListQuerySnapshots(_ context.Context, queryName, fleet, endpointID string) ([]pgstore.QuerySnapshotRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []pgstore.QuerySnapshotRecord
	for _, snap := range m.snapshots {
		if snap.QueryName != queryName {
			continue
		}
		if fleet != "" && snap.Fleet != fleet {
			continue
		}
		if endpointID != "" && snap.EndpointID != endpointID {
			continue
		}
		out = append(out, snap)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EndpointID < out[j].EndpointID })
	return out, nil
}

func (m *MemoryQueryStore) ListQueryDiffs(_ context.Context, queryName, fleet, endpointID string, since time.Time, limit int) ([]pgstore.QueryDiffRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var out []pgstore.QueryDiffRecord
	for _, list := range m.diffs {
		for _, d := range list {
			if d.QueryName != queryName {
				continue
			}
			if fleet != "" && d.Fleet != fleet {
				continue
			}
			if endpointID != "" && d.EndpointID != endpointID {
				continue
			}
			if !since.IsZero() && d.CreatedAt.Before(since) {
				continue
			}
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryQueryStore) CreateAdhocQueryRequest(_ context.Context, req pgstore.AdhocQueryRequestRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.adhocReqs[req.RequestID] = req
	return nil
}

func (m *MemoryQueryStore) GetAdhocQueryRequest(_ context.Context, requestID string) (pgstore.AdhocQueryRequestRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	req, ok := m.adhocReqs[requestID]
	if !ok {
		return pgstore.AdhocQueryRequestRecord{}, pgstore.ErrAdhocQueryNotFound
	}
	return req, nil
}

func (m *MemoryQueryStore) ListPendingAdhocQueryRequests(_ context.Context, now time.Time, endpointID, fleet string) ([]pgstore.AdhocQueryRequestRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []pgstore.AdhocQueryRequestRecord
	for _, req := range m.adhocReqs {
		if !req.ExpiresAt.After(now) {
			continue
		}
		if req.EndpointID != "" && req.EndpointID != endpointID {
			continue
		}
		if req.EndpointID == "" && req.Fleet != fleet {
			continue
		}
		if resMap, ok := m.adhocResults[req.RequestID]; ok {
			if _, reported := resMap[endpointID]; reported {
				continue
			}
		}
		out = append(out, req)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *MemoryQueryStore) RecordAdhocQueryResult(_ context.Context, res pgstore.AdhocQueryResultRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.adhocResults[res.RequestID] == nil {
		m.adhocResults[res.RequestID] = map[string]pgstore.AdhocQueryResultRecord{}
	}
	m.adhocResults[res.RequestID][res.EndpointID] = res
	return nil
}

func (m *MemoryQueryStore) ListAdhocQueryResults(_ context.Context, requestID string) ([]pgstore.AdhocQueryResultRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	resMap := m.adhocResults[requestID]
	out := make([]pgstore.AdhocQueryResultRecord, 0, len(resMap))
	for _, r := range resMap {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EndpointID < out[j].EndpointID })
	return out, nil
}

func (m *MemoryQueryStore) PruneExpiredAdhocQueries(_ context.Context, cutoff time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, req := range m.adhocReqs {
		if req.ExpiresAt.Before(cutoff) {
			delete(m.adhocReqs, id)
			delete(m.adhocResults, id)
		}
	}
	return nil
}
