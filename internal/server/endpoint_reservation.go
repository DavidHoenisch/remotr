package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/registry"
	pgstore "github.com/DavidHoenisch/remotr/internal/store/postgres"
)

type endpointReservationRequest struct {
	Fleet      string `json:"fleet"`
	EndpointID string `json:"endpoint_id"`
	TTL        int64  `json:"ttl_seconds"`
}

type endpointReservationResponse struct {
	EndpointID string    `json:"endpoint_id"`
	Fleet      string    `json:"fleet"`
	Token      string    `json:"token"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func (s *Server) endpointReservations() (registry.EndpointReservations, bool) {
	if s.cfg.Admin == nil {
		return nil, false
	}
	res, ok := s.cfg.Admin.(registry.EndpointReservations)
	return res, ok
}

func (s *Server) handleCreateEndpointReservation(w http.ResponseWriter, r *http.Request) {
	res, ok := s.endpointReservations()
	if !ok {
		http.Error(w, "endpoint reservations unavailable", http.StatusServiceUnavailable)
		return
	}
	var req endpointReservationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.Fleet = strings.TrimSpace(req.Fleet)
	req.EndpointID = strings.TrimSpace(req.EndpointID)
	if err := configrepo.ValidateFleetName(req.Fleet); err != nil {
		http.Error(w, "invalid fleet", http.StatusBadRequest)
		return
	}
	if err := identity.ValidateEndpointID(req.EndpointID); err != nil {
		http.Error(w, "invalid endpoint_id: "+err.Error(), http.StatusBadRequest)
		return
	}
	ttl := time.Duration(req.TTL) * time.Second
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		http.Error(w, "reservation failed", http.StatusInternalServerError)
		return
	}
	token := hex.EncodeToString(raw)
	expires := time.Now().UTC().Add(ttl)

	completeMutation, ok := s.beginFastPathMutationHTTP(w, mutationEnrollmentToken, "")
	if !ok {
		return
	}
	defer completeMutation()
	if err := res.ReserveEndpoint(req.EndpointID, req.Fleet, token, expires); err != nil {
		switch {
		case errors.Is(err, registry.ErrEndpointEnrolled):
			http.Error(w, "endpoint "+req.EndpointID+" is already enrolled and cannot be reserved", http.StatusConflict)
		case errors.Is(err, registry.ErrReservationFleetConflict):
			http.Error(w, "endpoint "+req.EndpointID+" is already reserved in another fleet; unreserve it first", http.StatusConflict)
		default:
			slog.Error("reserve endpoint", "endpoint_id", req.EndpointID, "fleet", req.Fleet, "err", err)
			http.Error(w, "reservation failed", http.StatusInternalServerError)
		}
		return
	}
	annotateAudit(r, audit.ActionAdminEndpointReserve, "endpoint", req.EndpointID, auditDetails(
		audit.PublicDetail("fleet", req.Fleet),
		audit.MetadataDetail("expires_at", expires.Format(time.RFC3339)),
	))
	writeJSON(w, endpointReservationResponse{EndpointID: req.EndpointID, Fleet: req.Fleet, Token: token, ExpiresAt: expires})
}

type endpointUnreserveResponse struct {
	EndpointID string `json:"endpoint_id"`
	Forced     bool   `json:"forced,omitempty"`
	Warning    string `json:"warning,omitempty"`
}

// releasedOverrideConsequence explains why releasing a reservation whose
// override is in the current release breaks Git sync.
func releasedOverrideConsequence(id, releaseRef string) string {
	return fmt.Sprintf("endpoint %s has an override in release %s (endpoints/%s/); unreserving will block Git sync until that directory is removed from the configuration repository", id, releaseRef, id)
}

func (s *Server) releasedEndpointOverride(ctx context.Context, id string) (string, bool, error) {
	if s.cfg.ArtifactStore == nil {
		return "", false, nil
	}
	ref := s.releaseRef(ctx)
	_, _, err := s.cfg.ArtifactStore.GetCompiledArtifactForEndpoint(ctx, id, ref, "desired")
	if err == nil {
		return ref, true, nil
	}
	if errors.Is(err, pgstore.ErrCompiledArtifactNotFound) {
		return "", false, nil
	}
	return "", false, err
}

func (s *Server) handleDeleteEndpointReservation(w http.ResponseWriter, r *http.Request) {
	res, ok := s.endpointReservations()
	if !ok {
		http.Error(w, "endpoint reservations unavailable", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	if err := identity.ValidateEndpointID(id); err != nil {
		http.Error(w, "invalid endpoint id", http.StatusBadRequest)
		return
	}
	force := r.URL.Query().Get("force") == "true"

	ep, found, err := s.cfg.Admin.GetEndpoint(id)
	if err != nil {
		slog.Error("unreserve lookup", "endpoint_id", id, "err", err)
		http.Error(w, "unreserve failed", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "endpoint "+id+" is not reserved", http.StatusNotFound)
		return
	}
	if !ep.Reserved() {
		http.Error(w, "endpoint "+id+" is enrolled; only unclaimed reservations can be unreserved (use endpoint delete)", http.StatusConflict)
		return
	}
	ref, released, err := s.releasedEndpointOverride(r.Context(), id)
	if err != nil {
		slog.Error("unreserve artifact lookup", "endpoint_id", id, "err", err)
		http.Error(w, "unreserve failed: could not check the current release for an override", http.StatusInternalServerError)
		return
	}
	resp := endpointUnreserveResponse{EndpointID: id}
	if released {
		consequence := releasedOverrideConsequence(id, ref)
		if !force {
			http.Error(w, consequence+". Remove that directory first, or re-run with --force to unreserve anyway.", http.StatusConflict)
			return
		}
		resp.Forced = true
		resp.Warning = consequence + "."
	}

	completeMutation, ok := s.beginFastPathMutationHTTP(w, mutationEnrollmentToken, "")
	if !ok {
		return
	}
	defer completeMutation()
	if err := res.UnreserveEndpoint(id); err != nil {
		switch {
		case errors.Is(err, registry.ErrReservationNotFound):
			http.Error(w, "endpoint "+id+" is not reserved", http.StatusNotFound)
		case errors.Is(err, registry.ErrEndpointEnrolled):
			http.Error(w, "endpoint "+id+" is enrolled; only unclaimed reservations can be unreserved (use endpoint delete)", http.StatusConflict)
		default:
			slog.Error("unreserve endpoint", "endpoint_id", id, "err", err)
			http.Error(w, "unreserve failed", http.StatusInternalServerError)
		}
		return
	}
	details := []executor.SafeField{audit.PublicDetail("fleet", ep.Fleet)}
	if resp.Forced {
		details = append(details, audit.PublicDetail("forced", "true"), audit.PublicDetail("release_ref", ref))
	}
	annotateAudit(r, audit.ActionAdminEndpointUnreserve, "endpoint", id, auditDetails(details...))
	writeJSON(w, resp)
}
