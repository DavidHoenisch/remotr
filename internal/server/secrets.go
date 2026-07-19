package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

func (s *Server) handleResolveSecret(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Secrets == nil {
		http.Error(w, "secret provider unavailable", http.StatusServiceUnavailable)
		return
	}
	endpointID, err := endpointIDFromRequest(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	endpoint, ok := s.cfg.Registry.EndpointByID(endpointID)
	if !ok {
		http.Error(w, "unknown endpoint", http.StatusForbidden)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var request secrets.ResolveRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil && err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	provider, _, err := secrets.ParseReference(request.Reference)
	if err != nil || provider != secrets.ProviderRemotr || secrets.ValidateRequest(request) != nil {
		http.Error(w, "invalid secret resolution request", http.StatusBadRequest)
		return
	}

	releaseRef := s.releaseRef(r.Context())
	artifact, digest, err := resolveDesiredArtifact(r.Context(), s.cfg.ArtifactStore, s.cfg.ConfigRepoPath, endpoint.Fleet, endpointID, releaseRef)
	if err != nil {
		http.Error(w, "active artifact unavailable", http.StatusServiceUnavailable)
		return
	}
	if request.ArtifactDigest == "" || request.ArtifactDigest != digest {
		http.Error(w, "secret resolution unauthorized", http.StatusForbidden)
		return
	}
	state, _, err := models.ParseStateWithDiagnostics(bytes.NewReader(artifact))
	if err != nil || !secrets.ArtifactAuthorizes(state, request.ResourceAddress, request.Reference, request.Purpose) {
		http.Error(w, "secret resolution unauthorized", http.StatusForbidden)
		return
	}

	request.EndpointID = endpointID
	request.Fleet = endpoint.Fleet
	request.ArtifactDigest = digest
	resolved, err := s.cfg.Secrets.Resolve(r.Context(), request)
	if err != nil {
		if errors.Is(err, secrets.ErrUnauthorized) {
			http.Error(w, "secret resolution unauthorized", http.StatusForbidden)
			return
		}
		http.Error(w, "secret resolution failed", http.StatusServiceUnavailable)
		return
	}
	if len(resolved.Material) == 0 || len(resolved.Material) > secrets.MaxMaterialBytes {
		http.Error(w, "secret resolution failed", http.StatusServiceUnavailable)
		return
	}

	annotateAudit(r, audit.ActionAgentSecretResolve, "secret", request.Reference, auditDetails(
		audit.PublicDetail("endpoint_id", endpointID),
		audit.FingerprintDetail("artifact_digest", digest),
		audit.PublicDetail("resource_address", request.ResourceAddress),
		audit.PublicDetail("purpose", request.Purpose),
		audit.PublicDetail("provider", resolved.Provider),
		audit.SecretReferenceDetail("version", resolved.Version),
		audit.FingerprintDetail("fingerprint", resolved.Fingerprint),
	))
	writeJSON(w, resolved)
}
