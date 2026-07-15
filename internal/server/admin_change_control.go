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
	authorization, err := s.cfg.ChangeControl.AuthorizeRollout(chi.URLParam(r, "id"), changecontrol.RolloutSpec{
		ValidFrom: body.ValidFrom, ValidUntil: body.ValidUntil, AttemptLimit: body.AttemptLimit,
		MaxConcurrency: body.MaxConcurrency, ExecutionWindows: body.ExecutionWindows,
	}, changeControlActor(r), body.Justification)
	if err != nil {
		writeChangeControlError(w, err)
		return
	}
	annotateAudit(r, audit.ActionAdminChangeAuthorize, "change_request", authorization.ChangeRequestID, map[string]any{"justification": body.Justification})
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
	switch action {
	case "pause":
		request, err = s.cfg.ChangeControl.Pause(id, actor)
		annotateAudit(r, audit.ActionAdminChangePause, "change_request", id, nil)
	case "resume":
		request, err = s.cfg.ChangeControl.Resume(id, actor)
		annotateAudit(r, audit.ActionAdminChangeResume, "change_request", id, nil)
	case "revoke":
		request, err = s.cfg.ChangeControl.Revoke(id, actor)
		annotateAudit(r, audit.ActionAdminChangeRevoke, "change_request", id, nil)
	default:
		err = fmt.Errorf("unsupported lifecycle action")
	}
	if err != nil {
		writeChangeControlError(w, err)
		return
	}
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
	annotateAudit(r, audit.ActionAdminBaselinePromote, "baseline", baseline.ID, map[string]any{"resource_address": body.ResourceAddress})
	writeJSON(w, baseline)
}

func (s *Server) handleCreateBaselineAdoption(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ChangeControl == nil {
		http.Error(w, "change control unavailable", http.StatusServiceUnavailable)
		return
	}
	var plan changecontrol.FleetPlan
	if err := json.NewDecoder(r.Body).Decode(&plan); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	plan.Fleet = chi.URLParam(r, "fleet")
	request, err := s.cfg.ChangeControl.CreateBaselineAdoption(plan, changeControlActor(r))
	if err != nil {
		writeChangeControlError(w, err)
		return
	}
	annotateAudit(r, audit.ActionAdminBaselineAdopt, "change_request", request.ID, map[string]any{"fleet": plan.Fleet})
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
