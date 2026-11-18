package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/registry"
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
