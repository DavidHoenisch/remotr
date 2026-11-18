package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/pki"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

// v1 enrollment accepts an optional CSR. When csr_pem is present the agent keeps its
// private key locally and the response omits key_pem. Without csr_pem the server generates
// the key pair for backward compatibility.

type enrollRequest struct {
	Token      string `json:"token"`
	CSRPEM     string `json:"csr_pem,omitempty"`
	EndpointID string `json:"endpoint_id,omitempty"`
}

type enrollResponse struct {
	EndpointID string `json:"endpoint_id"`
	CertPEM    string `json:"cert_pem"`
	KeyPEM     string `json:"key_pem,omitempty"`
	CAPEM      string `json:"ca_pem"`
}

func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Enroller == nil || s.cfg.CACert == nil || s.cfg.CAKey == nil {
		http.Error(w, "enrollment unavailable", http.StatusServiceUnavailable)
		return
	}

	var req enrollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.Token == "" {
		http.Error(w, "token required", http.StatusBadRequest)
		return
	}

	var reservations registry.EndpointReservations
	var fleet, endpointID string
	if res, isRes := s.cfg.Enroller.(registry.EndpointReservations); isRes {
		if boundID, bound := res.EnrollmentTokenBinding(req.Token); bound {
			reservations, endpointID = res, boundID
		}
	}
	if reservations == nil {
		redeemed, ok := s.cfg.Enroller.RedeemEnrollmentToken(req.Token)
		if !ok {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}
		fleet = redeemed
		resolved, err := resolveEnrollEndpointID(req.EndpointID)
		if err != nil {
			http.Error(w, "invalid endpoint_id", http.StatusBadRequest)
			return
		}
		endpointID = resolved
	}
	completeMutation, ok := s.beginFastPathMutationHTTP(w, mutationEndpointEnrollment, endpointID)
	if !ok {
		return
	}
	defer completeMutation()

	resp := enrollResponse{EndpointID: endpointID, CAPEM: string(s.cfg.CACertPEM)}
	var fingerprint string
	if req.CSRPEM != "" {
		signed, err := pki.SignEndpointCSR(s.cfg.CACert, s.cfg.CAKey, []byte(req.CSRPEM), endpointID)
		if err != nil {
			http.Error(w, "invalid csr", http.StatusBadRequest)
			return
		}
		fingerprint = identity.Fingerprint(signed.Cert)
		resp.CertPEM = string(signed.CertPEM)
	} else {
		cred, err := pki.IssueEndpointCredential(s.cfg.CACert, s.cfg.CAKey, endpointID)
		if err != nil {
			http.Error(w, "enrollment failed", http.StatusInternalServerError)
			return
		}
		fingerprint = identity.Fingerprint(cred.Cert)
		resp.CertPEM = string(cred.CertPEM)
		resp.KeyPEM = string(cred.KeyPEM)
	}

	details := []executor.SafeField{}
	if reservations != nil {
		claimedFleet, err := reservations.ClaimReservedEndpoint(req.Token, endpointID, fingerprint)
		if err != nil {
			if errors.Is(err, registry.ErrBoundTokenInvalid) {
				http.Error(w, "invalid or expired token", http.StatusUnauthorized)
				return
			}
			slog.Error("enroll claim reserved endpoint", "endpoint_id", endpointID, "err", err)
			http.Error(w, "enrollment failed", http.StatusInternalServerError)
			return
		}
		fleet = claimedFleet
		details = append(details, audit.PublicDetail("reservation", "claimed"))
		if requested := strings.TrimSpace(req.EndpointID); requested != "" && requested != endpointID {
			details = append(details, audit.MetadataDetail("ignored_requested_endpoint_id", requested))
		}
	} else {
		ep := registry.Endpoint{ID: endpointID, Fleet: fleet, CertFingerprint: fingerprint}
		if err := s.cfg.Enroller.RegisterEndpoint(ep); err != nil {
			slog.Error("enroll register endpoint", "endpoint_id", endpointID, "fleet", fleet, "err", err)
			http.Error(w, enrollRegisterErrMessage(err), enrollRegisterStatus(err))
			return
		}
	}

	details = append(details, audit.PublicDetail("fleet", fleet))
	annotateAudit(r, audit.ActionAgentEnroll, "endpoint", endpointID, auditDetails(details...))
	writeJSON(w, resp)
}

func resolveEnrollEndpointID(requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		return identity.ResolveEndpointID(requested)
	}
	return identity.RandomEndpointID("ep")
}
