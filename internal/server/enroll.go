package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

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

	fleet, ok := s.cfg.Enroller.RedeemEnrollmentToken(req.Token)
	if !ok {
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}

	endpointID, err := resolveEnrollEndpointID(req.EndpointID)
	if err != nil {
		http.Error(w, "invalid endpoint_id", http.StatusBadRequest)
		return
	}

	if req.CSRPEM != "" {
		signed, err := pki.SignEndpointCSR(s.cfg.CACert, s.cfg.CAKey, []byte(req.CSRPEM), endpointID)
		if err != nil {
			http.Error(w, "invalid csr", http.StatusBadRequest)
			return
		}

		ep := registry.Endpoint{
			ID:              endpointID,
			Fleet:           fleet,
			CertFingerprint: identity.Fingerprint(signed.Cert),
		}
		if err := s.cfg.Enroller.RegisterEndpoint(ep); err != nil {
			http.Error(w, "enrollment failed", http.StatusInternalServerError)
			return
		}

		writeJSON(w, enrollResponse{
			EndpointID: endpointID,
			CertPEM:    string(signed.CertPEM),
			CAPEM:      string(s.cfg.CACertPEM),
		})
		return
	}

	cred, err := pki.IssueEndpointCredential(s.cfg.CACert, s.cfg.CAKey, endpointID)
	if err != nil {
		http.Error(w, "enrollment failed", http.StatusInternalServerError)
		return
	}

	ep := registry.Endpoint{
		ID:              endpointID,
		Fleet:           fleet,
		CertFingerprint: identity.Fingerprint(cred.Cert),
	}
	if err := s.cfg.Enroller.RegisterEndpoint(ep); err != nil {
		http.Error(w, "enrollment failed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, enrollResponse{
		EndpointID: endpointID,
		CertPEM:    string(cred.CertPEM),
		KeyPEM:     string(cred.KeyPEM),
		CAPEM:      string(s.cfg.CACertPEM),
	})
}

func resolveEnrollEndpointID(requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		return identity.ResolveEndpointID(requested)
	}
	return identity.RandomEndpointID("ep")
}
