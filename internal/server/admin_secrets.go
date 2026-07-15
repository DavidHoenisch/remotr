package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

func (s *Server) handleUploadSecretVersion(w http.ResponseWriter, r *http.Request) {
	if s.cfg.SecretRegistry == nil {
		http.Error(w, "secret registry unavailable", http.StatusServiceUnavailable)
		return
	}
	if contentType := strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]); contentType != "application/octet-stream" {
		http.Error(w, "secret upload requires application/octet-stream", http.StatusUnsupportedMediaType)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, secrets.MaxMaterialBytes+1)
	material, err := io.ReadAll(r.Body)
	if err != nil || len(material) == 0 || len(material) > secrets.MaxMaterialBytes {
		clear(material)
		http.Error(w, "secret material is empty or exceeds the upload limit", http.StatusBadRequest)
		return
	}
	metadata, err := s.cfg.SecretRegistry.Upload(r.Context(), secrets.UploadRequest{
		Name: r.URL.Query().Get("name"), Fleet: r.URL.Query().Get("fleet"), EndpointID: r.URL.Query().Get("endpoint_id"),
		Material: material, ActorID: changeControlActor(r),
	})
	clear(material)
	if err != nil {
		http.Error(w, "secret upload rejected", http.StatusBadRequest)
		return
	}
	annotateAudit(r, audit.ActionAdminSecretUpload, "secret_version", metadata.Name+"@"+metadata.Version, map[string]any{
		"name": metadata.Name, "version": metadata.Version, "fingerprint": metadata.Fingerprint,
		"fleet": metadata.Fleet, "endpoint_id": metadata.EndpointID, "active": false,
	})
	writeJSONStatus(w, http.StatusCreated, metadata)
}

func (s *Server) handleListSecretVersions(w http.ResponseWriter, r *http.Request) {
	if s.cfg.SecretRegistry == nil {
		http.Error(w, "secret registry unavailable", http.StatusServiceUnavailable)
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		http.Error(w, "secret name is required", http.StatusBadRequest)
		return
	}
	metadata, err := s.cfg.SecretRegistry.ListMetadata(r.Context(), name)
	if err != nil {
		http.Error(w, "list secret versions failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, metadata)
}

func (s *Server) handleSecretPlaintextReadDenied(w http.ResponseWriter, r *http.Request) {
	annotateAudit(r, audit.ActionAdminSecretReadDenied, "secret_version", r.URL.Query().Get("name")+"@"+r.URL.Query().Get("version"), nil)
	w.Header().Set("Allow", "")
	http.Error(w, "operator plaintext retrieval is not supported", http.StatusMethodNotAllowed)
}

type secretVersionLifecycleRequest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (s *Server) handleActivateSecretVersion(w http.ResponseWriter, r *http.Request) {
	if s.cfg.SecretRegistry == nil {
		http.Error(w, "secret registry unavailable", http.StatusServiceUnavailable)
		return
	}
	var body secretVersionLifecycleRequest
	if err := decodeSecretLifecycleRequest(w, r, &body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	uses, err := s.secretActivationUses(r, body.Name)
	if err != nil {
		http.Error(w, "secret reference planning failed", http.StatusServiceUnavailable)
		return
	}
	metadata, err := s.cfg.SecretRegistry.Activate(r.Context(), secrets.ActivationRequest{Name: body.Name, Version: body.Version, ActorID: changeControlActor(r), Uses: uses})
	if err != nil {
		http.Error(w, "secret activation rejected", secretLifecycleStatus(err))
		return
	}
	annotateAudit(r, audit.ActionAdminSecretActivate, "secret_version", metadata.Name+"@"+metadata.Version, map[string]any{
		"activation_generation": metadata.ActivationGeneration, "rollouts": metadata.Rollouts,
	})
	writeJSON(w, metadata)
}

func (s *Server) handleRevokeSecretVersion(w http.ResponseWriter, r *http.Request) {
	if s.cfg.SecretRegistry == nil {
		http.Error(w, "secret registry unavailable", http.StatusServiceUnavailable)
		return
	}
	var body secretVersionLifecycleRequest
	if err := decodeSecretLifecycleRequest(w, r, &body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	metadata, err := s.cfg.SecretRegistry.Revoke(r.Context(), secrets.RevokeRequest{Name: body.Name, Version: body.Version, ActorID: changeControlActor(r)})
	if err != nil {
		http.Error(w, "secret revocation rejected", secretLifecycleStatus(err))
		return
	}
	annotateAudit(r, audit.ActionAdminSecretRevoke, "secret_version", metadata.Name+"@"+metadata.Version, map[string]any{
		"resolution_blocked": true, "endpoint_copy_status": metadata.EndpointCopyStatus,
	})
	writeJSON(w, metadata)
}

func decodeSecretLifecycleRequest(w http.ResponseWriter, r *http.Request, target *secretVersionLifecycleRequest) error {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if strings.TrimSpace(target.Name) == "" || strings.TrimSpace(target.Version) == "" {
		return errors.New("name and version are required")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("request contains trailing JSON")
	}
	return nil
}

func secretLifecycleStatus(err error) int {
	if errors.Is(err, secrets.ErrVersionNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, secrets.ErrVersionRevoked) {
		return http.StatusConflict
	}
	return http.StatusBadRequest
}

func writeJSONStatus(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Server) secretActivationUses(r *http.Request, name string) ([]secrets.ActivationUse, error) {
	if s.cfg.Admin == nil {
		return []secrets.ActivationUse{}, nil
	}
	fleets, err := s.cfg.Admin.ListFleets()
	if err != nil {
		return nil, err
	}
	endpoints, err := s.cfg.Admin.ListEndpoints()
	if err != nil {
		return nil, err
	}
	targets := make(map[string][]string)
	for _, endpoint := range endpoints {
		targets[endpoint.Fleet] = append(targets[endpoint.Fleet], endpoint.ID)
	}
	for fleet := range targets {
		sort.Strings(targets[fleet])
	}
	reference := "remotr:" + name + "@active"
	releaseRef := s.releaseRef(r.Context())
	var uses []secrets.ActivationUse
	for _, fleet := range fleets {
		artifact, digest, err := resolveDesiredArtifact(r.Context(), s.cfg.ArtifactStore, s.cfg.ConfigRepoPath, fleet, "", releaseRef)
		if err != nil {
			return nil, err
		}
		state, _, err := models.ParseStateWithDiagnostics(strings.NewReader(string(artifact)))
		if err != nil {
			return nil, err
		}
		uses = append(uses, activationUsesFromState(state, reference, fleet, releaseRef, digest, targets[fleet])...)
	}
	return uses, nil
}

func activationUsesFromState(state models.State, reference, fleet, releaseRef, digest string, endpointIDs []string) []secrets.ActivationUse {
	var uses []secrets.ActivationUse
	appendUse := func(configuration, resource, purpose string, risk models.RiskClass, provider string) {
		uses = append(uses, secrets.ActivationUse{
			Fleet: fleet, ResourceAddress: models.ResourceAddress(configuration, resource), Purpose: purpose,
			Risk: risk, Provider: provider, ReleaseRef: releaseRef, ArtifactDigest: digest, EndpointIDs: append([]string(nil), endpointIDs...),
		})
	}
	for _, configuration := range state.Configurations {
		for _, repository := range configuration.APTRepositories {
			if repository.CredentialRef == reference {
				appendUse(configuration.Name, repository.Name, "repository-credential", repository.EffectiveRisk(models.RiskSensitive), "apt")
			}
		}
		for _, user := range configuration.Users {
			if user.PasswordHashRef == reference {
				appendUse(configuration.Name, user.Name, "password-hash", user.EffectiveRisk(models.RiskAccess), "user")
			}
		}
		for _, schedule := range configuration.EndpointSchedules {
			for _, variable := range schedule.Environment {
				if variable.SecretRef == reference {
					appendUse(configuration.Name, schedule.Name, "schedule-environment", schedule.EffectiveRisk(models.RiskSensitive), string(schedule.Backend))
					break
				}
			}
		}
		for _, profile := range configuration.NetworkProfiles {
			if profile.CredentialRef == reference {
				appendUse(configuration.Name, profile.Name, "network-credential", profile.EffectiveRisk(models.RiskConnectivity), string(profile.Provider))
			}
		}
		for _, certificate := range configuration.Certificates {
			if certificate.CertificateRef == reference {
				appendUse(configuration.Name, certificate.Name, "certificate-public", certificate.EffectiveRisk(models.RiskSensitive), "certificate")
			}
			if certificate.PrivateKeyRef == reference {
				appendUse(configuration.Name, certificate.Name, "certificate-private-key", certificate.EffectiveRisk(models.RiskSensitive), "certificate")
			}
			for _, chainRef := range certificate.ChainRefs {
				if chainRef == reference {
					appendUse(configuration.Name, certificate.Name, "certificate-chain", certificate.EffectiveRisk(models.RiskSensitive), "certificate")
				}
			}
		}
	}
	return uses
}
