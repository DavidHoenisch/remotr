package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/identity"
)

func (s *Server) handleListChangeRequests(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.ChangeControl == nil {
		http.Error(w, "change control unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, s.cfg.ChangeControl.List())
}

func (s *Server) handleGetChangeRequest(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ChangeControl == nil {
		http.Error(w, "change control unavailable", http.StatusServiceUnavailable)
		return
	}
	request, ok := s.cfg.ChangeControl.Get(chi.URLParam(r, "id"))
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, request)
}

type authorizeChangeRequestBody struct {
	ValidFrom        time.Time                       `json:"valid_from,omitempty"`
	ValidUntil       time.Time                       `json:"valid_until,omitempty"`
	AttemptLimit     int                             `json:"attempt_limit"`
	MaxConcurrency   int                             `json:"max_concurrency"`
	ExecutionWindows []changecontrol.RecurringWindow `json:"execution_windows,omitempty"`
	Justification    string                          `json:"justification"`
}

func (s *Server) handleAuthorizeChangeRequest(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ChangeControl == nil {
		http.Error(w, "change control unavailable", http.StatusServiceUnavailable)
		return
	}
	var body authorizeChangeRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	changeRequestID := chi.URLParam(r, "id")
	authorization, err := s.cfg.ChangeControl.AuthorizeRollout(changeRequestID, changecontrol.RolloutSpec{
		ValidFrom: body.ValidFrom, ValidUntil: body.ValidUntil, AttemptLimit: body.AttemptLimit,
		MaxConcurrency: body.MaxConcurrency, ExecutionWindows: body.ExecutionWindows,
	}, changeControlActor(r), body.Justification)
	if err != nil {
		writeChangeControlError(w, err)
		return
	}
	annotateAudit(r, audit.ActionAdminChangeAuthorize, "change_request", changeRequestID, auditDetails(audit.PresenceDetail("justification", body.Justification != "")))
	writeJSON(w, authorization)
}

func (s *Server) handlePauseChangeRequest(w http.ResponseWriter, r *http.Request) {
	s.handleChangeLifecycle(w, r, "pause")
}
func (s *Server) handleResumeChangeRequest(w http.ResponseWriter, r *http.Request) {
	s.handleChangeLifecycle(w, r, "resume")
}
func (s *Server) handleRevokeChangeRequest(w http.ResponseWriter, r *http.Request) {
	s.handleChangeLifecycle(w, r, "revoke")
}

func (s *Server) handleChangeLifecycle(w http.ResponseWriter, r *http.Request, action string) {
	if s.cfg.ChangeControl == nil {
		http.Error(w, "change control unavailable", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	actor := changeControlActor(r)
	var request changecontrol.ChangeRequest
	var err error
	var auditAction string
	switch action {
	case "pause":
		request, err = s.cfg.ChangeControl.Pause(id, actor)
		auditAction = audit.ActionAdminChangePause
	case "resume":
		request, err = s.cfg.ChangeControl.Resume(id, actor)
		auditAction = audit.ActionAdminChangeResume
	case "revoke":
		request, err = s.cfg.ChangeControl.Revoke(id, actor)
		auditAction = audit.ActionAdminChangeRevoke
	default:
		err = fmt.Errorf("unsupported lifecycle action")
	}
	if err != nil {
		writeChangeControlError(w, err)
		return
	}
	annotateAudit(r, auditAction, "change_request", id, nil)
	writeJSON(w, request)
}

type promoteBaselineBody struct {
	ResourceAddress       string `json:"resource_address"`
	AcknowledgeExceptions bool   `json:"acknowledge_exceptions"`
}

func (s *Server) handlePromoteChangeBaseline(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ChangeControl == nil {
		http.Error(w, "change control unavailable", http.StatusServiceUnavailable)
		return
	}
	var body promoteBaselineBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	baseline, err := s.cfg.ChangeControl.PromoteBaselineWithOptions(chi.URLParam(r, "id"), body.ResourceAddress, changeControlActor(r), changecontrol.BaselinePromotionOptions{AcknowledgeExceptions: body.AcknowledgeExceptions})
	if err != nil {
		writeChangeControlError(w, err)
		return
	}
	annotateAudit(r, audit.ActionAdminBaselinePromote, "baseline", baseline.ID, auditDetails(audit.PublicDetail("resource_address", body.ResourceAddress)))
	writeJSON(w, baseline)
}

func (s *Server) handleRegenerateLegacyChangeRequest(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ChangeControl == nil {
		http.Error(w, "change control unavailable", http.StatusServiceUnavailable)
		return
	}
	var body struct{}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil && err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	legacyID := chi.URLParam(r, "id")
	legacy, ok := s.cfg.ChangeControl.Get(legacyID)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if legacy.LegacyMigration == nil {
		http.Error(w, "change request is already canonical", http.StatusBadRequest)
		return
	}
	derived, err := s.deriveBaselineAdoptionPlan(r.Context(), legacy.Fleet)
	if err != nil {
		writeChangeControlError(w, err)
		return
	}
	result, err := s.cfg.ChangeControl.RegenerateLegacyBaselineAdoption(legacyID, derived.Plan, derived.TrustedIdentities, changeControlActor(r))
	if err != nil {
		writeChangeControlError(w, err)
		return
	}
	annotateAudit(r, audit.ActionAdminChangeRegenerate, "change_request", legacyID, auditDetails(audit.PublicDetail("replacement_change_request_id", result.ReplacementRequest.ID)))
	writeJSON(w, result)
}

func (s *Server) handleCreateBaselineAdoption(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ChangeControl == nil {
		http.Error(w, "change control unavailable", http.StatusServiceUnavailable)
		return
	}
	var body struct{}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil && err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	derived, err := s.deriveBaselineAdoptionPlan(r.Context(), chi.URLParam(r, "fleet"))
	if err != nil {
		writeChangeControlError(w, err)
		return
	}
	request, err := s.cfg.ChangeControl.CreateCanonicalBaselineAdoption(derived.Plan, derived.TrustedIdentities, changeControlActor(r))
	if err != nil {
		writeChangeControlError(w, err)
		return
	}
	annotateAudit(r, audit.ActionAdminBaselineAdopt, "change_request", request.ID, auditDetails(audit.PublicDetail("fleet", derived.Plan.Fleet)))
	writeJSON(w, request)
}

func changeControlActor(r *http.Request) string {
	if actor := operatorIDFromContext(r.Context()); actor != "" {
		return actor
	}
	if id, err := identity.OperatorIDFromCert(peerCert(r)); err == nil {
		return id
	}
	return "operator"
}

func writeChangeControlError(w http.ResponseWriter, err error) {
	if changecontrol.IsPersistenceError(err) {
		http.Error(w, ErrChangeControlPersistenceUnavailable, http.StatusInternalServerError)
		return
	}
	http.Error(w, err.Error(), http.StatusBadRequest)
}

const ErrChangeControlPersistenceUnavailable = "change control persistence unavailable"
