package main

import (
	"context"
	"log/slog"

	"github.com/DavidHoenisch/remotr/internal/agent/cronexec"
	"github.com/DavidHoenisch/remotr/internal/agent/sync"
)

func (s *syncRunState) runDueCrons(ctx context.Context, resp sync.Response, pending *sync.Pending) {
	if len(resp.DueCrons) == 0 {
		return
	}
	if resp.CronsDigest != "" {
		pending.SetCronsDigest(resp.CronsDigest)
	}
	for _, due := range resp.DueCrons {
		slog.Info("cron due",
			"name", due.CronName,
			"runId", due.RunID,
			"scheduledFor", due.ScheduledFor,
		)
		result := cronexec.Run(ctx, due.SpecYAML, due.CronName, due.RunID, nil)
		failures := make([]sync.CronFailurePayload, len(result.Failures))
		for i, f := range result.Failures {
			failures[i] = sync.CronFailurePayload{
				ResourceAddress: f.ResourceAddress,
				Message:         f.Message,
			}
		}
		pending.AddCronResult(sync.CronResultPayload{
			RunID:       result.RunID,
			CronName:    result.CronName,
			Status:      result.Status,
			StartedAt:   result.StartedAt,
			CompletedAt: result.CompletedAt,
			Message:     result.Message,
			Failures:    failures,
		})
		if result.Status != "success" {
			slog.Error("cron failed", "cron", result.CronName, "err", result.Message)
		} else {
			slog.Info("cron complete", "cron", result.CronName)
		}
	}
}
