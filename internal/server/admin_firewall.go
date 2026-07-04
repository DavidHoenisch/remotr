package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/DavidHoenisch/remotr/internal/registry"
)

func (s *Server) handleGetEndpointFirewallAudit(w http.ResponseWriter, r *http.Request) {
	if s.cfg.StateReports == nil {
		http.Error(w, "firewall audit unavailable", http.StatusServiceUnavailable)
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}

	ep, ok := s.cfg.Registry.EndpointByID(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	report, ok, err := s.cfg.StateReports.GetEndpointFirewallAudit(r.Context(), id)
	if err != nil {
		http.Error(w, "get failed", http.StatusInternalServerError)
		return
	}
	if !ok {
		writeJSON(w, registry.FirewallAuditReport{EndpointID: ep.ID})
		return
	}
	writeJSON(w, report)
}
