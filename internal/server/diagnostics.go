package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/diagnostics"
	"github.com/DavidHoenisch/remotr/internal/executor"
)

type diagnosticUploadURLRequest struct {
	RequestID string `json:"requestId"`
}

type diagnosticUploadURLResponse struct {
	URL       string    `json:"url"`
	Key       string    `json:"key"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Server) handleDiagnosticUploadURL(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Diagnostics == nil || s.cfg.AppPackageBlobs == nil {
		http.Error(w, "diagnostics unavailable", http.StatusServiceUnavailable)
		return
	}
	endpointID, err := endpointIDFromRequest(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req diagnosticUploadURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.RequestID == "" {
		http.Error(w, "requestId required", http.StatusBadRequest)
		return
	}

	diagReq, ok, err := s.cfg.Diagnostics.GetDiagnosticRequest(r.Context(), req.RequestID)
	if err != nil {
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if diagReq.EndpointID != endpointID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch diagReq.Status {
	case diagnostics.StatusDispatched, diagnostics.StatusRunning:
	default:
		http.Error(w, "request not accepting upload", http.StatusConflict)
		return
	}

	ttl := s.cfg.AppPackagePresignTTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	url, err := s.cfg.AppPackageBlobs.PresignPut(r.Context(), diagReq.S3Key, ttl)
	if err != nil {
		http.Error(w, "upload url failed", http.StatusInternalServerError)
		return
	}

	if err := s.cfg.Diagnostics.MarkDiagnosticRunning(r.Context(), req.RequestID); err != nil {
		http.Error(w, "upload url failed", http.StatusInternalServerError)
		return
	}

	annotateAudit(r, audit.ActionAgentDiagnosticsUpload, "endpoint", endpointID, auditDetails(audit.PublicDetail("request_id", req.RequestID)))
	writeJSON(w, diagnosticUploadURLResponse{
		URL:       url,
		Key:       diagReq.S3Key,
		ExpiresAt: time.Now().UTC().Add(ttl),
	})
}

type diagnosticCollectionPayload struct {
	RequestID  string    `json:"requestId"`
	Collectors []string  `json:"collectors"`
	Since      time.Time `json:"since"`
	Until      time.Time `json:"until"`
}

type diagnosticResultPayload struct {
	RequestID string              `json:"requestId"`
	Status    string              `json:"status"`
	SHA256    string              `json:"sha256,omitempty"`
	SizeBytes int64               `json:"sizeBytes,omitempty"`
	Failure   *executor.SafeError `json:"failure,omitempty"`
}

func (s *Server) diagnosticCollectionForEndpoint(ctx context.Context, endpointID string) *diagnosticCollectionPayload {
	if s.cfg.Diagnostics == nil {
		return nil
	}
	req, ok, err := s.cfg.Diagnostics.PendingDiagnosticForEndpoint(ctx, endpointID)
	if err != nil || !ok {
		return nil
	}
	if req.Status == diagnostics.StatusPending {
		if err := s.cfg.Diagnostics.MarkDiagnosticDispatched(ctx, req.ID); err != nil {
			return nil
		}
	}
	return &diagnosticCollectionPayload{
		RequestID:  req.ID,
		Collectors: req.Spec.Collectors,
		Since:      req.Spec.Since,
		Until:      req.Spec.Until,
	}
}

func (s *Server) persistDiagnosticResult(ctx context.Context, endpointID string, result *diagnosticResultPayload) {
	if s.cfg.Diagnostics == nil || result == nil || result.RequestID == "" {
		return
	}
	diagReq, ok, err := s.cfg.Diagnostics.GetDiagnosticRequest(ctx, result.RequestID)
	if err != nil || !ok {
		return
	}
	if diagReq.EndpointID != endpointID {
		return
	}
	payload := diagnostics.ResultPayload{
		RequestID: result.RequestID,
		Status:    result.Status,
		SHA256:    result.SHA256,
		SizeBytes: result.SizeBytes,
		Failure:   result.Failure,
	}
	if payload.Status == diagnostics.StatusReady {
		candidate := diagReq
		candidate.SHA256 = payload.SHA256
		candidate.SizeBytes = payload.SizeBytes
		var validationErr error
		if s.cfg.AppPackageBlobs == nil {
			validationErr = diagnostics.ErrDiagnosticsUnavail
		} else {
			_, validationErr = s.readClassifiedDiagnosticBundle(ctx, candidate)
		}
		if validationErr != nil {
			payload.Status = diagnostics.StatusFailed
			payload.SHA256 = ""
			payload.SizeBytes = 0
			failure := executor.NewSafeError(executor.ReasonCode("diagnostic_bundle_invalid"), "diagnostic_validation", validationErr)
			payload.Failure = &failure
			if s.cfg.AppPackageBlobs != nil {
				if deleteErr := s.cfg.AppPackageBlobs.DeleteObject(ctx, diagReq.S3Key); deleteErr != nil {
					classified := executor.NewSafeError("diagnostic_cleanup_failed", "diagnostic_cleanup", deleteErr)
					slog.Warn("delete rejected diagnostic bundle", "request", result.RequestID, "failure", classified)
				}
			}
		}
	}
	if payload.Status != diagnostics.StatusReady && payload.Status != diagnostics.StatusFailed {
		payload.Status = diagnostics.StatusFailed
		if payload.Failure == nil {
			failure := executor.NewSafeError(executor.ReasonCode("diagnostic_status_invalid"), "diagnostic_result", nil)
			payload.Failure = &failure
		}
	}
	if err := s.cfg.Diagnostics.CompleteDiagnosticRequest(ctx, payload); err != nil {
		classified := executor.NewSafeError("diagnostic_persistence_failed", "diagnostic_persistence", err)
		slog.Warn("complete diagnostic request", "request", result.RequestID, "failure", classified)
	}
}
