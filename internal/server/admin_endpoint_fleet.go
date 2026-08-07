package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

type reassignEndpointRequest struct {
	Fleet string `json:"fleet"`
}

func (s *Server) handleReassignEndpoint(w http.ResponseWriter, r *http.Request) {
	reassigner, ok := s.cfg.Admin.(registry.EndpointReassigner)
	if !ok {
		http.Error(w, "endpoint reassignment unavailable", http.StatusServiceUnavailable)
		return
	}
	endpointID := chi.URLParam(r, "id")
	if err := identity.ValidateEndpointID(endpointID); err != nil {
		http.Error(w, "invalid endpoint id", http.StatusBadRequest)
		return
	}
	var request reassignEndpointRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := configrepo.ValidateFleetName(request.Fleet); err != nil {
		http.Error(w, "invalid fleet", http.StatusBadRequest)
		return
	}
	completeMutation, ok := s.beginFastPathMutationHTTP(w, mutationEndpointReassign, endpointID)
	if !ok {
		return
	}
	defer completeMutation()
	changed, err := reassigner.ReassignEndpoint(endpointID, request.Fleet)
	if err != nil {
		http.Error(w, "endpoint reassignment failed", http.StatusInternalServerError)
		return
	}
	if !changed {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	annotateAudit(r, audit.ActionAdminEndpointReassign, "endpoint", endpointID, auditDetails(audit.PublicDetail("fleet", request.Fleet)))
	writeJSON(w, request)
}
