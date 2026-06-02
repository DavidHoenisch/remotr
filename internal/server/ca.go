package server

import (
	"net/http"
)

// handleCAPEM serves the public Remotr CA certificate for endpoint and operator trust setup.
// No authentication — the CA cert is public key material, not a secret.
func (s *Server) handleCAPEM(w http.ResponseWriter, _ *http.Request) {
	if len(s.cfg.CACertPEM) == 0 {
		http.Error(w, "ca unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(s.cfg.CACertPEM)
}
