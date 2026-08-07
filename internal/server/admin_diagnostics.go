package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/diagnostics"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

type collectDiagnosticsRequest struct {
	Collectors []string  `json:"collectors,omitempty"`
	Since      time.Time `json:"since,omitempty"`
	Until      time.Time `json:"until,omitempty"`
}

type diagnosticRequestResponse struct {
	ID           string              `json:"id"`
	EndpointID   string              `json:"endpoint_id"`
	RequestedBy  string              `json:"requested_by,omitempty"`
	Status       string              `json:"status"`
	Spec         diagnostics.Spec    `json:"spec"`
	SHA256       string              `json:"sha256,omitempty"`
	SizeBytes    int64               `json:"size_bytes,omitempty"`
	Failure      *executor.SafeError `json:"failure,omitempty"`
	CreatedAt    time.Time           `json:"created_at"`
	DispatchedAt *time.Time          `json:"dispatched_at,omitempty"`
	CompletedAt  *time.Time          `json:"completed_at,omitempty"`
	ExpiresAt    time.Time           `json:"expires_at"`
}

func diagnosticRequestToResponse(req diagnostics.Request) diagnosticRequestResponse {
	return diagnosticRequestResponse{
		ID:           req.ID,
		EndpointID:   req.EndpointID,
		RequestedBy:  req.RequestedBy,
		Status:       req.Status,
		Spec:         req.Spec,
		SHA256:       req.SHA256,
		SizeBytes:    req.SizeBytes,
		Failure:      req.Failure,
		CreatedAt:    req.CreatedAt,
		DispatchedAt: req.DispatchedAt,
		CompletedAt:  req.CompletedAt,
		ExpiresAt:    req.ExpiresAt,
	}
}

func (s *Server) handleCollectDiagnostics(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Diagnostics == nil {
		http.Error(w, "diagnostics unavailable", http.StatusServiceUnavailable)
		return
	}
	if s.cfg.AppPackageBlobs == nil {
		http.Error(w, "diagnostics storage unavailable", http.StatusServiceUnavailable)
		return
	}

	id := chi.URLParam(r, "id")
	if err := identity.ValidateEndpointID(id); err != nil {
		http.Error(w, "invalid endpoint id", http.StatusBadRequest)
		return
	}

	var req collectDiagnosticsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	spec, err := diagnostics.NormalizeSpec(req.Collectors, req.Since, req.Until)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	completeMutation, ok := s.beginFastPathMutationHTTP(w, mutationDiagnostics, id)
	if !ok {
		return
	}
	defer completeMutation()

	requestedBy := operatorIDFromContext(r.Context())
	created, err := s.cfg.Diagnostics.CreateDiagnosticRequest(r.Context(), id, requestedBy, spec)
	if err != nil {
		if errors.Is(err, diagnostics.ErrActiveRequest) {
			http.Error(w, "endpoint already has an active diagnostic request", http.StatusConflict)
			return
		}
		if errors.Is(err, registry.ErrEndpointNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "collect request failed", http.StatusInternalServerError)
		return
	}

	annotateAudit(r, audit.ActionAdminDiagnosticsCollect, "endpoint", id, auditDetails(
		audit.PublicDetail("request_id", created.ID),
		audit.CountDetail("collectors", len(created.Spec.Collectors)),
	))
	writeJSON(w, diagnosticRequestToResponse(created))
}

func (s *Server) handleGetDiagnosticRequest(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Diagnostics == nil {
		http.Error(w, "diagnostics unavailable", http.StatusServiceUnavailable)
		return
	}
	requestID := chi.URLParam(r, "requestId")
	if requestID == "" {
		http.Error(w, "request id required", http.StatusBadRequest)
		return
	}
	_ = s.cfg.Diagnostics.ExpireDiagnosticRequests(r.Context())

	req, ok, err := s.cfg.Diagnostics.GetDiagnosticRequest(r.Context(), requestID)
	if err != nil {
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, diagnosticRequestToResponse(req))
}

func (s *Server) handleDownloadDiagnosticRequest(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Diagnostics == nil || s.cfg.AppPackageBlobs == nil {
		http.Error(w, "diagnostics unavailable", http.StatusServiceUnavailable)
		return
	}
	requestID := chi.URLParam(r, "requestId")
	if requestID == "" {
		http.Error(w, "request id required", http.StatusBadRequest)
		return
	}

	req, ok, err := s.cfg.Diagnostics.GetDiagnosticRequest(r.Context(), requestID)
	if err != nil {
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if req.Status != diagnostics.StatusReady {
		http.Error(w, "diagnostic bundle not ready", http.StatusConflict)
		return
	}
	if req.S3Key == "" {
		http.Error(w, "bundle unavailable", http.StatusNotFound)
		return
	}

	raw, err := s.readClassifiedDiagnosticBundle(r.Context(), req)
	if err != nil {
		classified := executor.NewSafeError("diagnostic_download_failed", "diagnostic_download", err)
		slog.Warn("diagnostics download", "request", requestID, "failure", classified)
		http.Error(w, "download failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", "attachment; filename=\"diagnostics-"+req.EndpointID+".tar.gz\"")
	if _, err := w.Write(raw); err != nil {
		classified := executor.NewSafeError("diagnostic_stream_failed", "diagnostic_stream", err)
		slog.Warn("diagnostics stream", "request", requestID, "failure", classified)
	}
}

func (s *Server) readClassifiedDiagnosticBundle(ctx context.Context, req diagnostics.Request) ([]byte, error) {
	body, objectSize, err := s.cfg.AppPackageBlobs.GetObject(ctx, req.S3Key)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	if objectSize <= 0 || objectSize > diagnostics.MaxBundleBytes || (req.SizeBytes > 0 && req.SizeBytes != objectSize) {
		return nil, errors.New("diagnostic bundle size is invalid")
	}
	raw, err := io.ReadAll(io.LimitReader(body, diagnostics.MaxBundleBytes+1))
	if err != nil || int64(len(raw)) != objectSize {
		return nil, errors.New("diagnostic bundle read is invalid")
	}
	if err := diagnostics.ValidateBundle(raw); err != nil {
		return nil, err
	}
	digest := sha256.Sum256(raw)
	if req.SHA256 == "" || req.SHA256 != hex.EncodeToString(digest[:]) {
		return nil, errors.New("diagnostic bundle digest is invalid")
	}
	return raw, nil
}
