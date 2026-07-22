package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
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
	scope, fleet, endpointID, err := secretUploadScope(r)
	if err != nil {
		http.Error(w, "secret upload rejected", http.StatusBadRequest)
		return
	}
	if allowed, err := s.authorizeSecretScope(r, http.MethodPost, scope, fleet, endpointID); err != nil || !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
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
		Name: r.URL.Query().Get("name"), Scope: scope, Fleet: fleet, EndpointID: endpointID,
		Material: material, ActorID: changeControlActor(r),
	})
	clear(material)
	if err != nil {
		http.Error(w, "secret upload rejected", http.StatusBadRequest)
		return
	}
	annotateAudit(r, audit.ActionAdminSecretUpload, "secret_version", metadata.Name+"@"+metadata.Version, auditDetails(
		audit.SecretReferenceDetail("name", metadata.Name),
		audit.SecretReferenceDetail("version", metadata.Version),
		audit.FingerprintDetail("fingerprint", metadata.Fingerprint),
		audit.PublicDetail("fleet", metadata.Fleet),
		audit.PublicDetail("endpoint_id", metadata.EndpointID),
		audit.PublicDetail("active", "false"),
	))
	writeJSONStatus(w, http.StatusCreated, metadata)
}

func secretUploadScope(r *http.Request) (secrets.Scope, string, string, error) {
	fleet := r.URL.Query().Get("fleet")
	endpointID := r.URL.Query().Get("endpoint_id")
	raw := r.URL.Query().Get("scope")
	if raw == "" {
		switch {
		case fleet != "" && endpointID == "":
			raw = string(secrets.ScopeFleet)
		case endpointID != "" && fleet == "":
			raw = string(secrets.ScopeEndpoint)
		}
	}
	scope, err := secrets.ParseScope(raw, fleet, endpointID)
	return scope, fleet, endpointID, err
}

func (s *Server) authorizeSecretScope(r *http.Request, method string, scope secrets.Scope, fleet, endpointID string) (bool, error) {
	if s.cfg.RBAC == nil {
		return true, nil
	}
	path := ""
	switch scope {
	case secrets.ScopeGlobal:
		path = "/v1/admin/secrets/global"
	case secrets.ScopeFleet:
		path = "/v1/admin/fleets/" + url.PathEscape(fleet) + "/secrets"
	case secrets.ScopeEndpoint:
		path = "/v1/admin/endpoints/" + url.PathEscape(endpointID) + "/secrets"
	default:
		return false, nil
	}
	return s.cfg.RBAC.Authorize(r.Context(), operatorIDFromContext(r.Context()), method, path)
}

func (s *Server) authorizeExistingSecretVersion(r *http.Request, method, name, version string) (bool, error) {
	if s.cfg.RBAC == nil {
		return true, nil
	}
	metadata, err := s.cfg.SecretRegistry.GetMetadata(r.Context(), name, version)
	if err != nil {
		return false, nil
	}
	return s.authorizeSecretScope(r, method, metadata.Scope, metadata.Fleet, metadata.EndpointID)
}

func (s *Server) handleListSecretVersions(w http.ResponseWriter, r *http.Request) {
	if s.cfg.SecretRegistry == nil {
		http.Error(w, "secret registry unavailable", http.StatusServiceUnavailable)
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		limit := 0
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil {
				http.Error(w, "invalid secret collection limit", http.StatusBadRequest)
				return
			}
			limit = parsed
		}
		page, err := s.listVisibleSecrets(r, r.URL.Query().Get("cursor"), limit)
		if err != nil {
			http.Error(w, "list secrets failed", http.StatusBadRequest)
			return
		}
		writeJSON(w, page)
		return
	}
	metadata, err := s.cfg.SecretRegistry.ListMetadata(r.Context(), name)
	if err != nil {
		http.Error(w, "list secret versions failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, metadata)
}

func (s *Server) listVisibleSecrets(r *http.Request, cursor string, limit int) (secrets.LogicalSecretPage, error) {
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 {
		return secrets.LogicalSecretPage{}, fmt.Errorf("secret collection limit must be between 1 and 100")
	}
	if s.cfg.RBAC == nil {
		return s.cfg.SecretRegistry.ListLogicalSecrets(r.Context(), cursor, limit)
	}
	operatorID := operatorIDFromContext(r.Context())
	if operatorID == "" {
		return secrets.LogicalSecretPage{}, fmt.Errorf("operator identity is unavailable")
	}
	items := make([]secrets.LogicalSecretSummary, 0, limit+1)
	scanCursor := cursor
	const scanLimit = 1000
	scanned := 0
	exhausted := false
	for len(items) <= limit && scanned < scanLimit {
		page, err := s.cfg.SecretRegistry.ListLogicalSecrets(r.Context(), scanCursor, 100)
		if err != nil {
			return secrets.LogicalSecretPage{}, err
		}
		for _, item := range page.Items {
			scanned++
			visible, err := s.secretSummaryVisible(r, operatorID, item)
			if err != nil {
				return secrets.LogicalSecretPage{}, err
			}
			if visible {
				items = append(items, item)
				if len(items) > limit {
					break
				}
			}
		}
		if len(items) > limit {
			break
		}
		if page.NextCursor == "" {
			exhausted = true
			break
		}
		scanCursor = page.NextCursor
	}
	if !exhausted && scanned >= scanLimit && len(items) <= limit {
		return secrets.LogicalSecretPage{}, fmt.Errorf("authorized secret collection exceeds scan bound")
	}
	out := secrets.LogicalSecretPage{Items: items}
	if len(items) > limit {
		out.Items = items[:limit]
		out.NextCursor = out.Items[len(out.Items)-1].Name
	}
	return out, nil
}

func (s *Server) secretSummaryVisible(r *http.Request, operatorID string, item secrets.LogicalSecretSummary) (bool, error) {
	path := ""
	switch item.Scope {
	case secrets.ScopeGlobal:
		path = "/v1/admin/secrets/global"
	case secrets.ScopeFleet:
		path = "/v1/admin/fleets/" + url.PathEscape(item.Fleet) + "/secrets"
	case secrets.ScopeEndpoint:
		path = "/v1/admin/endpoints/" + url.PathEscape(item.EndpointID) + "/secrets"
	default:
		return false, fmt.Errorf("stored secret scope is invalid")
	}
	return s.cfg.RBAC.Authorize(r.Context(), operatorID, http.MethodGet, path)
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
	if allowed, err := s.authorizeExistingSecretVersion(r, http.MethodPost, body.Name, body.Version); err != nil || !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	secretMetadata, err := s.cfg.SecretRegistry.GetMetadata(r.Context(), body.Name, body.Version)
	if err != nil {
		http.Error(w, "secret activation rejected", secretLifecycleStatus(err))
		return
	}
	uses, err := s.secretActivationUses(r, secretMetadata)
	if err != nil {
		http.Error(w, "secret reference planning failed", http.StatusServiceUnavailable)
		return
	}
	metadata, err := s.cfg.SecretRegistry.Activate(r.Context(), secrets.ActivationRequest{Name: body.Name, Version: body.Version, ActorID: changeControlActor(r), Uses: uses})
	if err != nil {
		http.Error(w, "secret activation rejected", secretLifecycleStatus(err))
		return
	}
	annotateAudit(r, audit.ActionAdminSecretActivate, "secret_version", metadata.Name+"@"+metadata.Version, auditDetails(
		audit.SecretReferenceDetail("activation_generation", strconv.FormatUint(metadata.ActivationGeneration, 10)),
		audit.CountDetail("rollouts", len(metadata.Rollouts)),
	))
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
	if allowed, err := s.authorizeExistingSecretVersion(r, http.MethodPost, body.Name, body.Version); err != nil || !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	metadata, err := s.cfg.SecretRegistry.Revoke(r.Context(), secrets.RevokeRequest{Name: body.Name, Version: body.Version, ActorID: changeControlActor(r)})
	if err != nil {
		http.Error(w, "secret revocation rejected", secretLifecycleStatus(err))
		return
	}
	annotateAudit(r, audit.ActionAdminSecretRevoke, "secret_version", metadata.Name+"@"+metadata.Version, auditDetails(
		audit.PublicDetail("resolution_blocked", "true"),
		audit.MetadataDetail("endpoint_copy_status", metadata.EndpointCopyStatus),
	))
	writeJSON(w, metadata)
}

func (s *Server) handleDeleteSecretVersion(w http.ResponseWriter, r *http.Request) {
	if s.cfg.SecretRegistry == nil {
		http.Error(w, "secret registry unavailable", http.StatusServiceUnavailable)
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	version := strings.TrimSpace(r.URL.Query().Get("version"))
	if name == "" || version == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if allowed, err := s.authorizeExistingSecretVersion(r, http.MethodDelete, name, version); err != nil || !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	abandonRecovery := false
	if raw := strings.TrimSpace(r.URL.Query().Get("abandon_recovery")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		abandonRecovery = parsed
	}
	if err := s.cfg.SecretRegistry.DeleteVersion(r.Context(), secrets.DeleteVersionRequest{
		Name: name, Version: version, ActorID: changeControlActor(r), AbandonRecovery: abandonRecovery,
	}); err != nil {
		http.Error(w, "secret deletion rejected", secretLifecycleStatus(err))
		return
	}
	annotateAudit(r, audit.ActionAdminSecretDelete, "secret_version", name+"@"+version, auditDetails(
		audit.PublicDetail("recovery_abandoned", strconv.FormatBool(abandonRecovery)),
	))
	w.WriteHeader(http.StatusNoContent)
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

const (
	maxSecretActivationFleets    = 4096
	maxSecretActivationConsumers = 16384
)

func (s *Server) secretActivationUses(r *http.Request, metadata secrets.VersionMetadata) ([]secrets.ActivationUse, error) {
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
	if len(fleets) > maxSecretActivationFleets {
		return nil, fmt.Errorf("secret consumer Fleet discovery exceeds bound")
	}
	targets := make(map[string][]string)
	for _, endpoint := range endpoints {
		targets[endpoint.Fleet] = append(targets[endpoint.Fleet], endpoint.ID)
	}
	for fleet := range targets {
		sort.Strings(targets[fleet])
	}
	switch metadata.Scope {
	case secrets.ScopeGlobal:
	case secrets.ScopeFleet:
		found := false
		for _, fleet := range fleets {
			if fleet == metadata.Fleet {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("secret Fleet scope is absent from active Fleet inventory")
		}
		fleets = []string{metadata.Fleet}
	case secrets.ScopeEndpoint:
		endpointFleet := ""
		for _, endpoint := range endpoints {
			if endpoint.ID == metadata.EndpointID {
				endpointFleet = endpoint.Fleet
				break
			}
		}
		if endpointFleet == "" {
			return nil, fmt.Errorf("secret endpoint scope is absent from active endpoint inventory")
		}
		fleets = []string{endpointFleet}
		targets[endpointFleet] = []string{metadata.EndpointID}
	default:
		return nil, fmt.Errorf("secret scope is invalid")
	}
	sort.Strings(fleets)
	reference := "remotr:" + metadata.Name + "@active"
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
		if len(uses) > maxSecretActivationConsumers {
			return nil, fmt.Errorf("secret consumer discovery exceeds bound")
		}
	}
	return uses, nil
}

func activationUsesFromState(state models.State, reference, fleet, releaseRef, digest string, endpointIDs []string) []secrets.ActivationUse {
	return activationUseIndexFromState(state, fleet, releaseRef, digest, endpointIDs)[reference]
}

func activationUseIndexFromState(state models.State, fleet, releaseRef, digest string, endpointIDs []string) map[string][]secrets.ActivationUse {
	usesByReference := make(map[string][]secrets.ActivationUse)
	appendUse := func(reference, configuration, resource, purpose string, risk models.RiskClass, provider string) {
		if strings.TrimSpace(reference) == "" {
			return
		}
		usesByReference[reference] = append(usesByReference[reference], secrets.ActivationUse{
			Fleet: fleet, ResourceAddress: models.ResourceAddress(configuration, resource), Purpose: purpose,
			Risk: risk, Provider: provider, ReleaseRef: releaseRef, ArtifactDigest: digest, EndpointIDs: append([]string(nil), endpointIDs...),
		})
	}
	for _, configuration := range state.Configurations {
		for _, repository := range configuration.APTRepositories {
			appendUse(repository.CredentialRef, configuration.Name, repository.Name, "repository-credential", repository.EffectiveRisk(models.RiskSensitive), "apt")
		}
		for _, repository := range configuration.PacmanRepositories {
			appendUse(repository.CredentialRef, configuration.Name, repository.Name, "repository-credential", repository.EffectiveRisk(models.RiskSensitive), "pacman")
		}
		for _, user := range configuration.Users {
			appendUse(user.PasswordHashRef, configuration.Name, user.Name, "password-hash", user.EffectiveRisk(models.RiskAccess), "user")
		}
		for _, schedule := range configuration.EndpointSchedules {
			seen := make(map[string]struct{})
			for _, variable := range schedule.Environment {
				if _, duplicate := seen[variable.SecretRef]; duplicate {
					continue
				}
				seen[variable.SecretRef] = struct{}{}
				appendUse(variable.SecretRef, configuration.Name, schedule.Name, "schedule-environment", schedule.EffectiveRisk(models.RiskSensitive), string(schedule.Backend))
			}
		}
		for _, profile := range configuration.NetworkProfiles {
			appendUse(profile.CredentialRef, configuration.Name, profile.Name, "network-credential", profile.EffectiveRisk(models.RiskConnectivity), string(profile.Provider))
		}
		for _, certificate := range configuration.Certificates {
			appendUse(certificate.CertificateRef, configuration.Name, certificate.Name, "certificate-public", certificate.EffectiveRisk(models.RiskSensitive), "certificate")
			appendUse(certificate.PrivateKeyRef, configuration.Name, certificate.Name, "certificate-private-key", certificate.EffectiveRisk(models.RiskSensitive), "certificate")
			for _, chainRef := range certificate.ChainRefs {
				appendUse(chainRef, configuration.Name, certificate.Name, "certificate-chain", certificate.EffectiveRisk(models.RiskSensitive), "certificate")
			}
		}
		for _, anchor := range configuration.TrustAnchors {
			appendUse(anchor.AnchorRef, configuration.Name, anchor.Name, "ca-trust-anchor", anchor.EffectiveRisk(models.RiskSensitive), "trust-anchor")
		}
		for _, subscription := range configuration.UbuntuPro {
			appendUse(subscription.TokenRef, configuration.Name, subscription.Name, "ubuntu-pro-token", subscription.ComputedRisk(), "ubuntu-pro")
		}
	}
	return usesByReference
}
