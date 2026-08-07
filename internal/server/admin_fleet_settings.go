package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/configrepo"
)

type setFleetRemediationPolicyRequest struct {
	Policy string `json:"policy"`
}

func (s *Server) handleSetFleetRemediationPolicy(w http.ResponseWriter, r *http.Request) {
	if s.cfg.FleetSettingsMutator == nil {
		http.Error(w, "fleet settings unavailable", http.StatusServiceUnavailable)
		return
	}
	fleet := chi.URLParam(r, "fleet")
	if err := configrepo.ValidateFleetName(fleet); err != nil {
		http.Error(w, "invalid fleet", http.StatusBadRequest)
		return
	}
	var request setFleetRemediationPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if request.Policy != "auto" && request.Policy != "report" {
		http.Error(w, "invalid remediation policy", http.StatusBadRequest)
		return
	}
	completeMutation, ok := s.beginFastPathMutationHTTP(w, mutationFleetPolicy, fleet)
	if !ok {
		return
	}
	defer completeMutation()
	if err := s.cfg.FleetSettingsMutator.SetRemediationPolicy(r.Context(), fleet, request.Policy); err != nil {
		http.Error(w, "fleet policy update failed", http.StatusInternalServerError)
		return
	}
	annotateAudit(r, audit.ActionAdminFleetPolicy, "fleet", fleet, auditDetails(audit.PublicDetail("policy", request.Policy)))
	writeJSON(w, setFleetRemediationPolicyRequest{Policy: request.Policy})
}
