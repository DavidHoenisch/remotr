package server

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

func (s *Server) handleGetEndpointCronReport(w http.ResponseWriter, r *http.Request) {
	if s.cfg.CronScheduler == nil {
		http.Error(w, "cron reports unavailable", http.StatusServiceUnavailable)
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

	jobs := s.cronJobStatuses(ep.Fleet, id, ep.Labels)
	report, ok, err := s.cfg.CronScheduler.GetEndpointCronReport(r.Context(), id, jobs)
	if err != nil {
		slog.Error("get endpoint cron report", "endpoint", id, "err", err)
		http.Error(w, "get failed", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	releaseRef := s.releaseRef(r.Context())
	_, digest, hasCrons, err := resolveCronsArtifact(r.Context(), s.cfg.ArtifactStore, ep.Fleet, id, releaseRef)
	if err == nil && hasCrons {
		report.CronsDigest = digest
	}
	writeJSON(w, report)
}

func (s *Server) handleGetFleetCronReport(w http.ResponseWriter, r *http.Request) {
	if s.cfg.CronScheduler == nil {
		http.Error(w, "cron reports unavailable", http.StatusServiceUnavailable)
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

	report, err := s.cfg.CronScheduler.ListFleetCronReports(r.Context(), fleet, func(endpointID string, labels map[string]string) []registry.CronJobStatus {
		ep, ok := s.cfg.Registry.EndpointByID(endpointID)
		if !ok {
			return nil
		}
		return s.cronJobStatuses(ep.Fleet, endpointID, labels)
	})
	if err != nil {
		slog.Error("list fleet cron reports", "fleet", fleet, "err", err)
		http.Error(w, "list failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, report)
}
