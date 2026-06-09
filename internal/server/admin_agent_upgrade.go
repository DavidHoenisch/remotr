package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/DavidHoenisch/remotr/internal/agentversion"
	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

type agentUpgradeRequest struct {
	Version string `json:"version"`
}

type agentUpgradeResponse struct {
	Version   string `json:"version"`
	Endpoints int    `json:"endpoints,omitempty"`
}

func (s *Server) handleEndpointAgentUpgrade(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Admin == nil {
		http.Error(w, "admin unavailable", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	if err := identity.ValidateEndpointID(id); err != nil {
		http.Error(w, "invalid endpoint id", http.StatusBadRequest)
		return
	}
	var req agentUpgradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	ver, err := agentversion.Normalize(req.Version)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.cfg.Admin.RequestAgentUpgrade(id, ver); err != nil {
		if errors.Is(err, registry.ErrEndpointNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "upgrade request failed", http.StatusInternalServerError)
		return
	}
	annotateAudit(r, audit.ActionAdminEndpointUpgrade, "endpoint", id, map[string]any{"version": ver})
	writeJSON(w, agentUpgradeResponse{Version: ver})
}

func (s *Server) handleFleetAgentUpgrade(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Admin == nil {
		http.Error(w, "admin unavailable", http.StatusServiceUnavailable)
		return
	}
	fleet := chi.URLParam(r, "fleet")
	if fleet == "" {
		http.Error(w, "fleet required", http.StatusBadRequest)
		return
	}
	var req agentUpgradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	ver, err := agentversion.Normalize(req.Version)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	n, err := s.cfg.Admin.RequestFleetAgentUpgrade(fleet, ver)
	if err != nil {
		http.Error(w, "upgrade request failed", http.StatusInternalServerError)
		return
	}
	annotateAudit(r, audit.ActionAdminFleetUpgrade, "fleet", fleet, map[string]any{
		"version":   ver,
		"endpoints": n,
	})
	writeJSON(w, agentUpgradeResponse{Version: ver, Endpoints: n})
}
