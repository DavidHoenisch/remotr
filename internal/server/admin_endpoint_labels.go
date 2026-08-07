package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/endpointlabel"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

type setEndpointLabelRequest struct {
	Value string `json:"value"`
}

type endpointLabelResponse struct {
	Key    string            `json:"key"`
	Value  string            `json:"value,omitempty"`
	Labels map[string]string `json:"labels"`
}

func (s *Server) handleSetEndpointLabel(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Admin == nil {
		http.Error(w, "admin unavailable", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	if err := identity.ValidateEndpointID(id); err != nil {
		http.Error(w, "invalid endpoint id", http.StatusBadRequest)
		return
	}
	key, err := url.PathUnescape(chi.URLParam(r, "key"))
	if err != nil || key == "" {
		http.Error(w, "label key required", http.StatusBadRequest)
		return
	}
	var req setEndpointLabelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := endpointlabel.ValidateKey(key); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := endpointlabel.ValidateValue(req.Value); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	completeMutation, ok := s.beginFastPathMutationHTTP(w, mutationEndpointLabels, id)
	if !ok {
		return
	}
	defer completeMutation()
	labels, err := s.cfg.Admin.SetEndpointLabel(id, key, req.Value)
	if err != nil {
		if errors.Is(err, registry.ErrEndpointNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	annotateAudit(r, audit.ActionAdminEndpointLabelSet, "endpoint", id, auditDetails(
		audit.PublicDetail("key", key),
		audit.PresenceDetail("value", req.Value != ""),
	))
	writeJSON(w, endpointLabelResponse{Key: key, Value: req.Value, Labels: labels})
}

func (s *Server) handleDeleteEndpointLabel(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Admin == nil {
		http.Error(w, "admin unavailable", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	if err := identity.ValidateEndpointID(id); err != nil {
		http.Error(w, "invalid endpoint id", http.StatusBadRequest)
		return
	}
	key, err := url.PathUnescape(chi.URLParam(r, "key"))
	if err != nil || key == "" {
		http.Error(w, "label key required", http.StatusBadRequest)
		return
	}
	if err := endpointlabel.ValidateKey(key); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	completeMutation, ok := s.beginFastPathMutationHTTP(w, mutationEndpointLabels, id)
	if !ok {
		return
	}
	defer completeMutation()
	removed, err := s.cfg.Admin.DeleteEndpointLabel(id, key)
	if err != nil {
		if errors.Is(err, registry.ErrEndpointNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !removed {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	annotateAudit(r, audit.ActionAdminEndpointLabelUnset, "endpoint", id, auditDetails(audit.PublicDetail("key", key)))
	w.WriteHeader(http.StatusNoContent)
}
