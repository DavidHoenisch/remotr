package server

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/pki"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

// v1 enrollment accepts an optional CSR. When csr_pem is present the agent keeps its
// private key locally and the response omits key_pem. Without csr_pem the server generates
// the key pair for backward compatibility.

type enrollRequest struct {
	Token  string `json:"token"`
	CSRPEM string `json:"csr_pem,omitempty"`
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

	endpointID, err := newEndpointID()
	if err != nil {
		http.Error(w, "enrollment failed", http.StatusInternalServerError)
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

func newEndpointID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
