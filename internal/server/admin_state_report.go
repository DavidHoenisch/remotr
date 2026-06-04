package server

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/DavidHoenisch/remotr/internal/configrepo"
)

func (s *Server) handleGetEndpointStateReport(w http.ResponseWriter, r *http.Request) {
	if s.cfg.StateReports == nil {
		http.Error(w, "state reports unavailable", http.StatusServiceUnavailable)
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}

	report, ok, err := s.cfg.StateReports.GetEndpointStateReport(r.Context(), id)
	if err != nil {
		slog.Error("get endpoint state report", "endpoint", id, "err", err)
		http.Error(w, "get failed", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, report)
}

func (s *Server) handleGetFleetStateReport(w http.ResponseWriter, r *http.Request) {
	if s.cfg.StateReports == nil {
		http.Error(w, "state reports unavailable", http.StatusServiceUnavailable)
		return
	}

	fleet := chi.URLParam(r, "fleet")
	if fleet == "" {
		http.Error(w, "fleet required", http.StatusBadRequest)
		return
	}
	if err := configrepo.ValidateFleetName(fleet); err != nil {
		http.Error(w, "invalid fleet", http.StatusBadRequest)
		return
	}

	report, err := s.cfg.StateReports.ListFleetStateReports(r.Context(), fleet)
	if err != nil {
		slog.Error("list fleet state reports", "fleet", fleet, "err", err)
		http.Error(w, "list failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, report)
}
