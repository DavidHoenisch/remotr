package server

import (
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/pki"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

type bootstrapRequest struct {
	Token string `json:"token"`
}

type bootstrapResponse struct {
	OperatorID string `json:"operator_id"`
	CertPEM    string `json:"cert_pem"`
	KeyPEM     string `json:"key_pem"`
	CAPEM      string `json:"ca_pem"`
}

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Admin == nil || s.cfg.Bootstrap == nil || s.cfg.CACert == nil || s.cfg.CAKey == nil {
		http.Error(w, "bootstrap unavailable", http.StatusServiceUnavailable)
		return
	}

	var req bootstrapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.Token == "" {
		http.Error(w, "token required", http.StatusBadRequest)
		return
	}
	if !s.cfg.Bootstrap.Valid(req.Token) {
		http.Error(w, "invalid bootstrap token", http.StatusUnauthorized)
		return
	}

	operatorID, err := newOperatorID()
	if err != nil {
		http.Error(w, "bootstrap failed", http.StatusInternalServerError)
		return
	}

	cred, err := pki.IssueOperatorCredential(s.cfg.CACert, s.cfg.CAKey, operatorID)
	if err != nil {
		http.Error(w, "bootstrap failed", http.StatusInternalServerError)
		return
	}

	fp := identity.Fingerprint(cred.Cert)
	if err := s.cfg.Admin.RegisterOperatorCredential(fp); err != nil {
		http.Error(w, "bootstrap failed", http.StatusInternalServerError)
		return
	}

	s.cfg.Bootstrap.Invalidate()

	writeJSON(w, bootstrapResponse{
		OperatorID: operatorID,
		CertPEM:    string(cred.CertPEM),
		KeyPEM:     string(cred.KeyPEM),
		CAPEM:      string(s.cfg.CACertPEM),
	})
}

type createEnrollTokenRequest struct {
	Fleet string `json:"fleet"`
	TTL   int64  `json:"ttl_seconds"`
}

type createEnrollTokenResponse struct {
	Token     string    `json:"token"`
	Fleet     string    `json:"fleet"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Server) handleCreateEnrollToken(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Admin == nil {
		http.Error(w, "admin unavailable", http.StatusServiceUnavailable)
		return
	}

	var req createEnrollTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.Fleet == "" {
		http.Error(w, "fleet required", http.StatusBadRequest)
		return
	}
	if err := configrepo.ValidateFleetName(req.Fleet); err != nil {
		http.Error(w, "invalid fleet", http.StatusBadRequest)
		return
	}

	ttl := time.Duration(req.TTL) * time.Second
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}

	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		http.Error(w, "token creation failed", http.StatusInternalServerError)
		return
	}
	token := hex.EncodeToString(raw)
	expires := time.Now().UTC().Add(ttl)

	if err := s.cfg.Admin.CreateEnrollmentToken(token, req.Fleet, expires); err != nil {
		http.Error(w, "token creation failed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, createEnrollTokenResponse{
		Token:     token,
		Fleet:     req.Fleet,
		ExpiresAt: expires,
	})
}

type endpointListItem struct {
	ID              string            `json:"id"`
	Fleet           string            `json:"fleet"`
	CertFingerprint string            `json:"cert_fingerprint,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

type driftSummaryItem struct {
	ReleaseRef string    `json:"release_ref"`
	Digest     string    `json:"digest"`
	ReportedAt time.Time `json:"reported_at"`
}

type endpointDetailItem struct {
	endpointListItem
	LastDrift *driftSummaryItem `json:"last_drift,omitempty"`
}

func endpointListItemFromRegistry(ep registry.Endpoint) endpointListItem {
	return endpointListItem{
		ID:              ep.ID,
		Fleet:           ep.Fleet,
		CertFingerprint: ep.CertFingerprint,
		Labels:          ep.Labels,
	}
}

func (s *Server) handleListEndpoints(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Admin == nil {
		http.Error(w, "admin unavailable", http.StatusServiceUnavailable)
		return
	}

	eps, err := s.cfg.Admin.ListEndpoints()
	if err != nil {
		http.Error(w, "list failed", http.StatusInternalServerError)
		return
	}

	out := make([]endpointListItem, 0, len(eps))
	for _, ep := range eps {
		out = append(out, endpointListItemFromRegistry(ep))
	}
	writeJSON(w, out)
}

func (s *Server) handleGetEndpoint(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Admin == nil {
		http.Error(w, "admin unavailable", http.StatusServiceUnavailable)
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}

	ep, ok, err := s.cfg.Admin.GetEndpoint(id)
	if err != nil {
		http.Error(w, "get failed", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	item := endpointDetailItem{endpointListItem: endpointListItemFromRegistry(ep)}
	if ep.LastDrift != nil {
		item.LastDrift = &driftSummaryItem{
			ReleaseRef: ep.LastDrift.ReleaseRef,
			Digest:     ep.LastDrift.Digest,
			ReportedAt: ep.LastDrift.ReportedAt,
		}
	}
	writeJSON(w, item)
}

func (s *Server) requireOperator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.Admin == nil {
			http.Error(w, "admin unavailable", http.StatusServiceUnavailable)
			return
		}
		cert := peerCert(r)
		if cert == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if _, err := identity.EndpointIDFromCert(cert); err == nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		fp := identity.Fingerprint(cert)
		if !s.cfg.Admin.IsOperatorCredential(fp) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func peerCert(r *http.Request) *x509.Certificate {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return nil
	}
	return r.TLS.PeerCertificates[0]
}

func newOperatorID() (string, error) {
	return newEndpointID()
}
