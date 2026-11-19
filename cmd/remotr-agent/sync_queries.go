package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/queryexec"
	"github.com/DavidHoenisch/remotr/internal/agent/sync"
)

func (s *syncRunState) runDispatchedQueries(ctx context.Context, client *sync.Client, resp sync.Response) {
	dispatched := make([]sync.DueQueryPayload, 0, len(resp.DueQueries)+len(resp.AdhocQueries))
	dispatched = append(dispatched, resp.DueQueries...)
	dispatched = append(dispatched, resp.AdhocQueries...)
	if len(dispatched) == 0 {
		return
	}

	exec := &queryexec.Executor{}
	results := make([]sync.QueryResultPayload, 0, len(dispatched))
	for _, dq := range dispatched {
		slog.Info("osquery dispatched", "name", dq.Name, "runId", dq.RunID, "trigger", dq.TriggerType)
		res := exec.Execute(ctx, queryexec.Request{
			RunID:       dq.RunID,
			RequestID:   dq.RequestID,
			TriggerType: dq.TriggerType,
			Name:        dq.Name,
			SQL:         dq.SQL,
			Timeout:     time.Duration(dq.TimeoutSec) * time.Second,
			MaxRows:     dq.MaxRows,
		})
		var sched *time.Time
		if !dq.ScheduledFor.IsZero() {
			t := dq.ScheduledFor
			sched = &t
		}
		results = append(results, sync.QueryResultPayload{
			RunID:        res.RunID,
			RequestID:    res.RequestID,
			TriggerType:  res.TriggerType,
			Name:         res.Name,
			Status:       res.Status,
			Rows:         res.Rows,
			RowCount:     res.RowCount,
			Truncated:    res.Truncated,
			DurationMS:   res.DurationMS,
			ErrorSummary: res.ErrorSummary,
			ScheduledFor: sched,
			CompletedAt:  res.CompletedAt,
		})
	}
	if client != nil {
		if err := client.PostQueryResults(ctx, results); err != nil {
			slog.Warn("post osquery results failed", "count", len(results), "err", err)
		}
	}
}
