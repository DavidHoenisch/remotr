package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/deploytoken"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

type createDeploymentTokenRequest struct {
	Label string `json:"label"`
	Fleet string `json:"fleet"`
	TTL   int64  `json:"ttl_seconds"`
}

type createDeploymentTokenResponse struct {
	Token     string    `json:"token"`
	Label     string    `json:"label"`
	Fleet     string    `json:"fleet"`
	ExpiresAt time.Time `json:"expires_at"`
}

type deploymentTokenItem struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	Fleet      string     `json:"fleet"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

func (s *Server) handleCreateDeploymentToken(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DeploymentTokens == nil {
		http.Error(w, "deployment tokens unavailable", http.StatusServiceUnavailable)
		return
	}

	var req createDeploymentTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := deploytoken.ValidateLabel(req.Label); err != nil {
		http.Error(w, "invalid label", http.StatusBadRequest)
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
		ttl = 365 * 24 * time.Hour
	}
	expires := time.Now().UTC().Add(ttl)

	meta, raw, err := s.cfg.DeploymentTokens.CreateDeploymentToken(req.Label, req.Fleet, expires)
	if err != nil {
		if errors.Is(err, registry.ErrDeploymentTokenLabelTaken) {
			http.Error(w, "label already exists", http.StatusConflict)
			return
		}
		http.Error(w, "token creation failed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, createDeploymentTokenResponse{
		Token:     raw,
		Label:     meta.Label,
		Fleet:     meta.Fleet,
		ExpiresAt: meta.ExpiresAt,
	})
}

func (s *Server) handleListDeploymentTokens(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DeploymentTokens == nil {
		http.Error(w, "deployment tokens unavailable", http.StatusServiceUnavailable)
		return
	}

	tokens, err := s.cfg.DeploymentTokens.ListDeploymentTokens()
	if err != nil {
		http.Error(w, "list failed", http.StatusInternalServerError)
		return
	}
	out := make([]deploymentTokenItem, 0, len(tokens))
	for _, tok := range tokens {
		out = append(out, deploymentTokenFromRegistry(tok))
	}
	writeJSON(w, out)
}

func (s *Server) handleGetDeploymentToken(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DeploymentTokens == nil {
		http.Error(w, "deployment tokens unavailable", http.StatusServiceUnavailable)
		return
	}

	label := chi.URLParam(r, "label")
	if err := deploytoken.ValidateLabel(label); err != nil {
		http.Error(w, "invalid label", http.StatusBadRequest)
		return
	}

	tok, ok, err := s.cfg.DeploymentTokens.GetDeploymentTokenByLabel(label)
	if err != nil {
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, deploymentTokenFromRegistry(tok))
}

func (s *Server) handleRevokeDeploymentToken(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DeploymentTokens == nil {
		http.Error(w, "deployment tokens unavailable", http.StatusServiceUnavailable)
		return
	}

	label := chi.URLParam(r, "label")
	if err := deploytoken.ValidateLabel(label); err != nil {
		http.Error(w, "invalid label", http.StatusBadRequest)
		return
	}

	revoked, err := s.cfg.DeploymentTokens.RevokeDeploymentToken(label)
	if err != nil {
		http.Error(w, "revoke failed", http.StatusInternalServerError)
		return
	}
	if !revoked {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func deploymentTokenFromRegistry(tok registry.DeploymentToken) deploymentTokenItem {
	return deploymentTokenItem{
		ID:         tok.ID,
		Label:      tok.Label,
		Fleet:      tok.Fleet,
		ExpiresAt:  tok.ExpiresAt,
		CreatedAt:  tok.CreatedAt,
		RevokedAt:  tok.RevokedAt,
		LastUsedAt: tok.LastUsedAt,
	}
}
