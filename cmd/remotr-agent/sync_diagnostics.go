package main

import (
	"context"
	"crypto/tls"
	"log/slog"

	"github.com/DavidHoenisch/remotr/internal/agent/diagnostics"
	"github.com/DavidHoenisch/remotr/internal/agent/sync"
	diagcatalog "github.com/DavidHoenisch/remotr/internal/diagnostics"
)

func (s *syncRunState) runDiagnosticCollection(
	ctx context.Context,
	resp sync.Response,
	pending *sync.Pending,
	currentVersion string,
	baseURL string,
	tlsCfg *tls.Config,
) {
	job := resp.DiagnosticCollection
	if job == nil || job.RequestID == "" {
		return
	}
	slog.Info("diagnostic collection requested", "requestId", job.RequestID)

	spec := diagcatalog.Spec{
		Collectors: job.Collectors,
		Since:      job.Since,
		Until:      job.Until,
	}
	bundle, err := diagnostics.Collect(ctx, diagnostics.Options{
		Spec:         spec,
		RequestID:    job.RequestID,
		AgentVersion: currentVersion,
		StateDir:     s.stateDir,
	})
	if err != nil {
		slog.Error("diagnostic collection failed", "requestId", job.RequestID, "err", err)
		pending.SetDiagnosticResult(sync.DiagnosticResultPayload{
			RequestID: job.RequestID,
			Status:    diagcatalog.StatusFailed,
			Message:   err.Error(),
		})
		return
	}

	uploader := diagnostics.NewUploadClient(baseURL, tlsCfg)
	if err := uploader.Upload(ctx, job.RequestID, bundle); err != nil {
		slog.Error("diagnostic upload failed", "requestId", job.RequestID, "err", err)
		pending.SetDiagnosticResult(sync.DiagnosticResultPayload{
			RequestID: job.RequestID,
			Status:    diagcatalog.StatusFailed,
			Message:   err.Error(),
		})
		return
	}

	slog.Info("diagnostic collection complete", "requestId", job.RequestID, "bytes", bundle.Size)
	pending.SetDiagnosticResult(sync.DiagnosticResultPayload{
		RequestID: job.RequestID,
		Status:    diagcatalog.StatusReady,
		SHA256:    bundle.SHA256,
		SizeBytes: bundle.Size,
	})
}
