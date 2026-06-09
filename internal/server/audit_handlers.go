package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/pki"
)

const auditExportPathPrefix = "/v1/exports/audit/"

type auditEventItem struct {
	ID               string         `json:"id"`
	OccurredAt       time.Time      `json:"occurred_at"`
	RequestID        string         `json:"request_id,omitempty"`
	ActorType        string         `json:"actor_type"`
	ActorID          string         `json:"actor_id,omitempty"`
	ActorFingerprint string         `json:"actor_fingerprint,omitempty"`
	Action           string         `json:"action"`
	Method           string         `json:"method"`
	Path             string         `json:"path"`
	StatusCode       int            `json:"status_code"`
	ResourceType     string         `json:"resource_type,omitempty"`
	ResourceID       string         `json:"resource_id,omitempty"`
	ClientIP         string         `json:"client_ip,omitempty"`
	Details          map[string]any `json:"details,omitempty"`
}

type auditEventPageResponse struct {
	Events     []auditEventItem `json:"events"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

type auditExportInfoResponse struct {
	ExportPath string `json:"export_path"`
	PathKey    string `json:"path_key"`
}

type createOperatorCredentialRequest struct {
	Label string   `json:"label,omitempty"`
	Roles []string `json:"roles,omitempty"`
}

type createOperatorCredentialResponse struct {
	OperatorID string   `json:"operator_id"`
	Label      string   `json:"label,omitempty"`
	Roles      []string `json:"roles,omitempty"`
	CertPEM    string   `json:"cert_pem"`
	KeyPEM     string   `json:"key_pem"`
	CAPEM      string   `json:"ca_pem"`
}

func (s *Server) handleListAuditEvents(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AuditLog == nil {
		http.Error(w, "audit log unavailable", http.StatusServiceUnavailable)
		return
	}

	filter, err := auditFilterFromRequest(r)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	page, err := s.cfg.AuditLog.ListAuditEvents(r.Context(), filter)
	if err != nil {
		http.Error(w, "list failed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, auditPageResponse(page))
}

func (s *Server) handleAuditExportInfo(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AuditLog == nil {
		http.Error(w, "audit log unavailable", http.StatusServiceUnavailable)
		return
	}

	pathKey, err := s.cfg.AuditLog.EnsureAuditExportPathKey(r.Context())
	if err != nil {
		http.Error(w, "export info unavailable", http.StatusInternalServerError)
		return
	}

	writeJSON(w, auditExportInfoResponse{
		ExportPath: auditExportPathPrefix + pathKey,
		PathKey:    pathKey,
	})
}

func (s *Server) handleExportAuditEvents(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AuditLog == nil {
		http.Error(w, "audit log unavailable", http.StatusServiceUnavailable)
		return
	}

	pathKey := chi.URLParam(r, "pathKey")
	expected, err := s.cfg.AuditLog.EnsureAuditExportPathKey(r.Context())
	if err != nil || pathKey == "" || pathKey != expected {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := s.authorizeOperatorRequest(r, r.Method, r.URL.Path); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if s.cfg.Admin != nil {
		cert := peerCert(r)
		if cert == nil || !s.cfg.Admin.IsOperatorCredential(identity.Fingerprint(cert)) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	filter, err := auditFilterFromRequest(r)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	page, err := s.cfg.AuditLog.ListAuditEvents(r.Context(), filter)
	if err != nil {
		http.Error(w, "export failed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, auditPageResponse(page))
}

func (s *Server) handleCreateOperatorCredential(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Admin == nil || s.cfg.CACert == nil || s.cfg.CAKey == nil {
		http.Error(w, "operator credential issuance unavailable", http.StatusServiceUnavailable)
		return
	}

	var req createOperatorCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	operatorID, err := newOperatorUUID()
	if err != nil {
		http.Error(w, "credential issuance failed", http.StatusInternalServerError)
		return
	}

	cred, err := pki.IssueOperatorCredential(s.cfg.CACert, s.cfg.CAKey, operatorID)
	if err != nil {
		http.Error(w, "credential issuance failed", http.StatusInternalServerError)
		return
	}

	fp := identity.Fingerprint(cred.Cert)
	if s.cfg.RBAC != nil {
		if err := s.cfg.RBAC.RegisterOperator(r.Context(), operatorID, fp, req.Roles); err != nil {
			http.Error(w, "credential issuance failed", http.StatusBadRequest)
			return
		}
	} else if err := s.cfg.Admin.RegisterOperatorCredential(fp); err != nil {
		http.Error(w, "credential issuance failed", http.StatusInternalServerError)
		return
	}

	details := map[string]any{"operator_id": operatorID, "roles": req.Roles}
	if req.Label != "" {
		details["label"] = req.Label
	}
	annotateAudit(r, audit.ActionAdminOperatorCreate, "operator", operatorID, details)

	writeJSON(w, createOperatorCredentialResponse{
		OperatorID: operatorID,
		Label:      req.Label,
		Roles:      req.Roles,
		CertPEM:    string(cred.CertPEM),
		KeyPEM:     string(cred.KeyPEM),
		CAPEM:      string(s.cfg.CACertPEM),
	})
}

func auditFilterFromRequest(r *http.Request) (audit.ListFilter, error) {
	filter := audit.ListFilter{
		Action:    r.URL.Query().Get("action"),
		ActorType: r.URL.Query().Get("actor_type"),
		Cursor:    r.URL.Query().Get("cursor"),
	}

	if since := r.URL.Query().Get("since"); since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return audit.ListFilter{}, err
		}
		filter.Since = t.UTC()
	}
	if until := r.URL.Query().Get("until"); until != "" {
		t, err := time.Parse(time.RFC3339, until)
		if err != nil {
			return audit.ListFilter{}, err
		}
		filter.Until = t.UTC()
	}
	if limitRaw := r.URL.Query().Get("limit"); limitRaw != "" {
		limit, err := strconv.Atoi(limitRaw)
		if err != nil || limit <= 0 {
			return audit.ListFilter{}, err
		}
		filter.Limit = limit
	}

	return filter, nil
}

func auditPageResponse(page audit.Page) auditEventPageResponse {
	out := auditEventPageResponse{
		Events:     make([]auditEventItem, 0, len(page.Events)),
		NextCursor: page.NextCursor,
	}
	for _, event := range page.Events {
		out.Events = append(out.Events, auditEventItem{
			ID:               event.ID,
			OccurredAt:       event.OccurredAt,
			RequestID:        event.RequestID,
			ActorType:        event.ActorType,
			ActorID:          event.ActorID,
			ActorFingerprint: event.ActorFingerprint,
			Action:           event.Action,
			Method:           event.Method,
			Path:             event.Path,
			StatusCode:       event.StatusCode,
			ResourceType:     event.ResourceType,
			ResourceID:       event.ResourceID,
			ClientIP:         event.ClientIP,
			Details:          event.Details,
		})
	}
	return out
}
