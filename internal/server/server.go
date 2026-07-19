package server

import (
	"compress/gzip"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	stdsync "sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/DavidHoenisch/remotr/internal/agent/sync"
	"github.com/DavidHoenisch/remotr/internal/apppackages"
	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

type Config struct {
	ConfigRepoPath       string
	ReleaseRef           string
	ReleaseRefSrc        ReleaseRefSource
	ArtifactStore        ArtifactStore
	Registry             registry.Registry
	Enroller             registry.Enroller
	Admin                registry.Admin
	DeploymentTokens     registry.DeploymentTokens
	Bootstrap            *Bootstrap
	FleetSettings        FleetSettings
	Telemetry            SyncTelemetry
	CronScheduler        CronScheduler
	StateReports         StateReports
	AuditLog             AuditLog
	RBAC                 RBAC
	GitWebhookPath       string
	GitWebhook           http.Handler
	GitSync              func(context.Context) error
	CACert               *x509.Certificate
	CAKey                crypto.PrivateKey
	CACertPEM            []byte
	GitHubRepo           string // agent self-upgrade release source (default DavidHoenisch/remotr)
	AppPackages          apppackages.Catalog
	AppPackageBlobs      *apppackages.BlobStore
	AppPackageURLs       apppackages.URLResolver
	AppPackagePresignTTL time.Duration
	Diagnostics          DiagnosticsStore
	SyncAdmission        SyncAdmission
	SyncMaxConcurrent    int
	SyncRetryAfter       time.Duration
	ChangeControl        *changecontrol.Registry
	Secrets              secrets.Resolver
	SecretRegistry       *secrets.RegistryService
	CapabilityDocuments  registry.CapabilityDocuments
	DeliveryStates       registry.DeliveryStates
	Now                  func() time.Time
}

type Server struct {
	cfg                 Config
	capabilityMu        stdsync.RWMutex
	currentCapabilities map[string]registry.CapabilityDocumentRecord
}

func New(cfg Config) *Server {
	if cfg.Registry == nil {
		cfg.Registry = registry.NewMemory()
	}
	if cfg.ArtifactStore == nil && strings.TrimSpace(cfg.ConfigRepoPath) != "" {
		cfg.ArtifactStore = &OnDemandArtifactResolver{RepoRoot: cfg.ConfigRepoPath}
	}
	if cfg.SyncAdmission == nil && cfg.SyncMaxConcurrent > 0 {
		cfg.SyncAdmission = newSyncLimiter(cfg.SyncMaxConcurrent, cfg.SyncRetryAfter)
	}
	if cfg.CapabilityDocuments == nil {
		cfg.CapabilityDocuments, _ = cfg.Registry.(registry.CapabilityDocuments)
	}
	if cfg.DeliveryStates == nil {
		cfg.DeliveryStates, _ = cfg.Registry.(registry.DeliveryStates)
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Server{cfg: cfg, currentCapabilities: make(map[string]registry.CapabilityDocumentRecord)}
}

type syncRequest struct {
	LastDigest         string                          `json:"lastDigest"`
	LastReleaseRef     string                          `json:"lastReleaseRef,omitempty"`
	Labels             map[string]string               `json:"labels,omitempty"`
	AgentVersion       string                          `json:"agentVersion,omitempty"`
	AgentUpgradeStatus *agentUpgradeStatusPayload      `json:"agentUpgradeStatus,omitempty"`
	Drift              *driftReportPayload             `json:"drift,omitempty"`
	ApplyFailure       *applyFailurePayload            `json:"applyFailure,omitempty"`
	CronResults        []cronResultPayload             `json:"cronResults,omitempty"`
	CronsDigest        string                          `json:"cronsDigest,omitempty"`
	SystemInfo         *systemInfoPayload              `json:"systemInfo,omitempty"`
	Usernames          []string                        `json:"usernames,omitempty"`
	DiagnosticResult   *diagnosticResultPayload        `json:"diagnosticResult,omitempty"`
	FirewallAudit      *firewallAuditPayload           `json:"firewallAudit,omitempty"`
	ChangePreflights   []changecontrol.PreflightReport `json:"changePreflights,omitempty"`
	RebootIntent       *sync.RebootIntentPayload       `json:"rebootIntent,omitempty"`
	NetworkIntent      *sync.NetworkIntentPayload      `json:"networkIntent,omitempty"`
	CapabilityDocument json.RawMessage                 `json:"capabilityDocument,omitempty"`
	stateReport        *registry.StateReportPayload
	capabilityDocument *capabilitydoc.Document
}

type syncResponse struct {
	Unchanged            bool                           `json:"unchanged"`
	ReleaseRef           string                         `json:"releaseRef,omitempty"`
	Digest               string                         `json:"digest,omitempty"`
	ArtifactYAML         []byte                         `json:"artifactYaml,omitempty"`
	RemediationPolicy    string                         `json:"remediationPolicy,omitempty"`
	AgentUpgrade         *agentUpgradePayload           `json:"agentUpgrade,omitempty"`
	DueCrons             []dueCronPayload               `json:"dueCrons,omitempty"`
	CronsDigest          string                         `json:"cronsDigest,omitempty"`
	DiagnosticCollection *diagnosticCollectionPayload   `json:"diagnosticCollection,omitempty"`
	ExecutionLeases      []changecontrol.ExecutionLease `json:"executionLeases,omitempty"`
	RebootAcknowledged   string                         `json:"rebootAcknowledged,omitempty"`
	NetworkAcknowledged  string                         `json:"networkAcknowledged,omitempty"`
	CapabilityBlocked    *sync.CapabilityBlocked        `json:"capabilityBlocked,omitempty"`
}

func validateRebootIntent(intent *sync.RebootIntentPayload) error {
	if intent == nil {
		return nil
	}
	if strings.TrimSpace(intent.Generation) == "" || len(intent.Generation) > 256 {
		return fmt.Errorf("reboot generation is required")
	}
	if intent.Phase != "awaiting-acknowledgement" {
		return fmt.Errorf("reboot intent phase is invalid")
	}
	if strings.TrimSpace(intent.PriorBootID) == "" || len(intent.PriorBootID) > 128 {
		return fmt.Errorf("prior boot identity is required")
	}
	if intent.NotBefore.IsZero() {
		return fmt.Errorf("reboot not-before timestamp is required")
	}
	if !intent.Deadline.IsZero() && !intent.Deadline.After(intent.NotBefore) {
		return fmt.Errorf("reboot deadline must follow not-before")
	}
	return nil
}

func validateNetworkIntent(intent *sync.NetworkIntentPayload, now time.Time) error {
	if intent == nil {
		return nil
	}
	if strings.TrimSpace(intent.ID) == "" || len(intent.ID) > 256 {
		return fmt.Errorf("network transaction id is required")
	}
	if intent.Phase != "awaiting-acknowledgement" || !intent.WatchdogArmed {
		return fmt.Errorf("network transaction is not armed for acknowledgement")
	}
	if intent.Deadline.IsZero() || !intent.Deadline.After(now.UTC()) {
		return fmt.Errorf("network transaction acknowledgement deadline elapsed")
	}
	if !strings.HasPrefix(intent.PlanHash, "sha256:") || len(intent.PlanHash) != len("sha256:")+64 {
		return fmt.Errorf("network transaction plan hash is invalid")
	}
	return nil
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(s.auditMiddleware)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/v1/ca.pem", s.handleCAPEM)
	r.Post("/v1/enroll", s.handleEnroll)
	r.With(gzipMiddleware).Post("/v1/sync", s.handleSync)
	r.Post("/v1/secrets/resolve", s.handleResolveSecret)
	r.Post("/v1/app-packages/download-url", s.handleAppPackageDownloadURL)
	r.Post("/v1/diagnostics/upload-url", s.handleDiagnosticUploadURL)
	r.Post("/v1/admin/bootstrap", s.handleBootstrap)
	r.Group(func(r chi.Router) {
		r.Use(s.requireOperator)
		r.Use(s.requirePermission)
		r.Get("/v1/admin/me", s.handleOperatorMe)
		r.Get("/v1/admin/rbac/roles", s.handleListRBACRoles)
		r.Post("/v1/admin/rbac/roles", s.handleCreateRBACRole)
		r.Get("/v1/admin/rbac/roles/{name}", s.handleGetRBACRole)
		r.Delete("/v1/admin/rbac/roles/{name}", s.handleDeleteRBACRole)
		r.Post("/v1/admin/rbac/roles/{name}/rules", s.handleCreateRBACRule)
		r.Delete("/v1/admin/rbac/roles/{name}/rules/{ruleID}", s.handleDeleteRBACRule)
		r.Get("/v1/admin/operators", s.handleListOperators)
		r.Put("/v1/admin/operators/{operator_id}/roles", s.handleSetOperatorRoles)
		r.Get("/v1/admin/fleets", s.handleListFleets)
		r.Get("/v1/admin/endpoints", s.handleListEndpoints)
		r.Get("/v1/admin/endpoints/{id}", s.handleGetEndpoint)
		r.Get("/v1/admin/endpoints/{id}/state-report", s.handleGetEndpointStateReport)
		r.Get("/v1/admin/endpoints/{id}/cron-report", s.handleGetEndpointCronReport)
		r.Get("/v1/admin/endpoints/{id}/firewall-audit", s.handleGetEndpointFirewallAudit)
		r.Delete("/v1/admin/endpoints/{id}", s.handleDeleteEndpoint)
		r.Put("/v1/admin/endpoints/{id}/labels/{key}", s.handleSetEndpointLabel)
		r.Delete("/v1/admin/endpoints/{id}/labels/{key}", s.handleDeleteEndpointLabel)
		r.Post("/v1/admin/endpoints/{id}/agent-upgrade", s.handleEndpointAgentUpgrade)
		r.Post("/v1/admin/endpoints/{id}/diagnostics/collect", s.handleCollectDiagnostics)
		r.Get("/v1/admin/diagnostics/{requestId}", s.handleGetDiagnosticRequest)
		r.Get("/v1/admin/diagnostics/{requestId}/download", s.handleDownloadDiagnosticRequest)
		r.Post("/v1/admin/fleets/{fleet}/agent-upgrade", s.handleFleetAgentUpgrade)
		r.Get("/v1/admin/fleets/{fleet}/state-report", s.handleGetFleetStateReport)
		r.Get("/v1/admin/fleets/{fleet}/cron-report", s.handleGetFleetCronReport)
		r.Get("/v1/admin/change-requests", s.handleListChangeRequests)
		r.Get("/v1/admin/change-requests/{id}", s.handleGetChangeRequest)
		r.Post("/v1/admin/change-requests/{id}/authorize", s.handleAuthorizeChangeRequest)
		r.Post("/v1/admin/change-requests/{id}/pause", s.handlePauseChangeRequest)
		r.Post("/v1/admin/change-requests/{id}/resume", s.handleResumeChangeRequest)
		r.Post("/v1/admin/change-requests/{id}/revoke", s.handleRevokeChangeRequest)
		r.Post("/v1/admin/change-requests/{id}/regenerate", s.handleRegenerateLegacyChangeRequest)
		r.Post("/v1/admin/change-requests/{id}/baseline", s.handlePromoteChangeBaseline)
		r.Post("/v1/admin/fleets/{fleet}/baseline-adoptions", s.handleCreateBaselineAdoption)
		r.Post("/v1/admin/enroll-tokens", s.handleCreateEnrollToken)
		r.Post("/v1/admin/deployment-tokens", s.handleCreateDeploymentToken)
		r.Get("/v1/admin/deployment-tokens", s.handleListDeploymentTokens)
		r.Get("/v1/admin/deployment-tokens/{label}", s.handleGetDeploymentToken)
		r.Delete("/v1/admin/deployment-tokens/{label}", s.handleRevokeDeploymentToken)
		r.Post("/v1/admin/app-packages", s.handleCreateAppPackage)
		r.Post("/v1/admin/app-packages/upload", s.handleUploadAppPackage)
		r.Get("/v1/admin/app-packages", s.handleListAppPackages)
		r.Get("/v1/admin/app-packages/detail", s.handleGetAppPackage)
		r.Delete("/v1/admin/app-packages/detail", s.handleDeleteAppPackage)
		r.Get("/v1/admin/secrets", s.handleListSecretVersions)
		r.Post("/v1/admin/secrets/versions", s.handleUploadSecretVersion)
		r.Get("/v1/admin/secrets/value", s.handleSecretPlaintextReadDenied)
		r.Post("/v1/admin/secrets/activate", s.handleActivateSecretVersion)
		r.Post("/v1/admin/secrets/revoke", s.handleRevokeSecretVersion)
		if s.cfg.GitSync != nil {
			r.Post("/v1/admin/git-sync", s.handleGitSync)
		}
		r.Get("/v1/admin/audit-events", s.handleListAuditEvents)
		r.Get("/v1/admin/audit-export", s.handleAuditExportInfo)
		r.Post("/v1/admin/operator-credentials", s.handleCreateOperatorCredential)
	})
	if s.cfg.AuditLog != nil {
		r.Get("/v1/exports/audit/{pathKey}", s.handleExportAuditEvents)
	}
	if s.cfg.GitWebhook != nil {
		r.Post("/v1/webhooks/git", s.cfg.GitWebhook.ServeHTTP)
		if path := s.cfg.GitWebhookPath; path != "" && path != "/v1/webhooks/git" && path != "/v1/admin/git-sync" {
			r.Post(path, s.cfg.GitWebhook.ServeHTTP)
		}
	}
	return r
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	endpointID, err := endpointIDFromRequest(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ep, ok := s.cfg.Registry.EndpointByID(endpointID)
	if !ok {
		http.Error(w, "unknown endpoint", http.StatusForbidden)
		return
	}
	if s.cfg.SyncAdmission != nil {
		release, retryAfter, admitted := s.cfg.SyncAdmission.Acquire()
		if !admitted {
			writeSyncOverload(w, retryAfter)
			return
		}
		if release != nil {
			defer release()
		}
	}

	var req syncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := admitCapabilityDocument(&req); err != nil {
		if isKnownModernCapabilityDocumentVersion(req.AgentVersion) {
			s.clearCurrentCapabilityEvidence(endpointID)
			releaseRef := s.releaseRef(r.Context())
			if err := s.acknowledgeOfferedArtifact(r.Context(), endpointID, req); err != nil {
				http.Error(w, "delivery state unavailable", http.StatusServiceUnavailable)
				return
			}
			missing := []sync.MissingRequirement{{ID: "capability-document", Revision: "1"}}
			unmanaged := !s.endpointHasActiveArtifact(r.Context(), ep)
			if err := s.recordCapabilityBlock(r.Context(), ep, releaseRef, missing, unmanaged); err != nil {
				http.Error(w, "delivery state unavailable", http.StatusServiceUnavailable)
				return
			}
			writeJSON(w, syncResponse{ReleaseRef: releaseRef, CapabilityBlocked: &sync.CapabilityBlocked{
				TargetReleaseRef: releaseRef, Unmanaged: unmanaged,
				MissingRequirements: missing,
			}})
			return
		}
		http.Error(w, "invalid capability document", http.StatusBadRequest)
		return
	}
	if err := validateRebootIntent(req.RebootIntent); err != nil {
		http.Error(w, "invalid reboot intent", http.StatusBadRequest)
		return
	}
	if err := validateNetworkIntent(req.NetworkIntent, time.Now()); err != nil {
		http.Error(w, "invalid network intent", http.StatusBadRequest)
		return
	}
	if err := admitStateReport(&req); err != nil {
		http.Error(w, "invalid state report", http.StatusBadRequest)
		return
	}
	if req.capabilityDocument == nil {
		s.clearCurrentCapabilityEvidence(endpointID)
	}
	if err := s.persistCurrentCapabilityDocument(r.Context(), endpointID, req); err != nil {
		slog.Warn("persist capability document", "endpoint", endpointID, "err", err)
		http.Error(w, "capability persistence unavailable", http.StatusServiceUnavailable)
		return
	}

	releaseRef := s.releaseRef(r.Context())
	if err := s.acknowledgeOfferedArtifact(r.Context(), endpointID, req); err != nil {
		http.Error(w, "delivery state unavailable", http.StatusServiceUnavailable)
		return
	}
	if req.capabilityDocument == nil {
		modern := isKnownModernCapabilityDocumentVersion(req.AgentVersion)
		persistedModern, err := s.hasPersistedCapabilityDocument(r.Context(), endpointID)
		if err != nil {
			http.Error(w, "capability persistence unavailable", http.StatusServiceUnavailable)
			return
		}
		if modern || persistedModern {
			missing := []sync.MissingRequirement{{ID: "capability-document", Revision: "1"}}
			unmanaged := !s.endpointHasActiveArtifact(r.Context(), ep)
			if err := s.recordCapabilityBlock(r.Context(), ep, releaseRef, missing, unmanaged); err != nil {
				http.Error(w, "delivery state unavailable", http.StatusServiceUnavailable)
				return
			}
			writeJSON(w, syncResponse{
				ReleaseRef: releaseRef,
				CapabilityBlocked: &sync.CapabilityBlocked{
					TargetReleaseRef:    releaseRef,
					MissingRequirements: missing,
					Unmanaged:           unmanaged,
				},
			})
			return
		}
	}

	var artifact []byte
	var digest string
	selectedSchemaVersion := 0
	selectionDocument := req.capabilityDocument
	if selectionDocument == nil {
		if legacyDocument, known := knownLegacyCapabilityDocument(req.AgentVersion); known {
			selectionDocument = &legacyDocument
		} else {
			minimalDocument := minimalLegacyCapabilityDocument()
			selectionDocument = &minimalDocument
		}
	}
	if selectionDocument != nil {
		selected, missing, compatible, selectErr := resolveCompatibleDesiredArtifact(r.Context(), s.cfg.ArtifactStore, s.cfg.ConfigRepoPath, ep.Fleet, endpointID, releaseRef, *selectionDocument)
		if selectErr != nil {
			slog.Error("resolve desired artifact variants", "endpoint", endpointID, "fleet", ep.Fleet, "release_ref", releaseRef, "err", selectErr)
			http.Error(w, "artifact unavailable", http.StatusInternalServerError)
			return
		}
		if !compatible {
			requirements := make([]sync.MissingRequirement, 0, len(missing))
			for _, requirement := range missing {
				requirements = append(requirements, sync.MissingRequirement{ID: requirement.ID, Revision: requirement.Revision})
			}
			unmanaged := !s.endpointHasActiveArtifact(r.Context(), ep)
			if err := s.recordCapabilityBlock(r.Context(), ep, releaseRef, requirements, unmanaged); err != nil {
				http.Error(w, "delivery state unavailable", http.StatusServiceUnavailable)
				return
			}
			writeJSON(w, syncResponse{ReleaseRef: releaseRef, CapabilityBlocked: &sync.CapabilityBlocked{
				TargetReleaseRef: releaseRef, MissingRequirements: requirements,
				Unmanaged: unmanaged,
			}})
			return
		}
		artifact, digest = selected.Artifact, selected.Digest
		selectedSchemaVersion = selected.SchemaVersion
	} else {
		artifact, digest, err = resolveDesiredArtifact(r.Context(), s.cfg.ArtifactStore, s.cfg.ConfigRepoPath, ep.Fleet, endpointID, releaseRef)
	}
	if err != nil {
		slog.Error("resolve desired artifact", "endpoint", endpointID, "fleet", ep.Fleet, "release_ref", releaseRef, "err", err)
		http.Error(w, "artifact unavailable", http.StatusInternalServerError)
		return
	}

	annotateAudit(r, audit.ActionAgentSync, "endpoint", endpointID, auditDetails(
		audit.PublicDetail("release_ref", releaseRef),
		audit.FingerprintDetail("digest", digest),
	))

	s.persistTelemetry(r.Context(), endpointID, releaseRef, req)
	s.persistAgentUpgradeTelemetry(r.Context(), endpointID, req)
	s.persistDiagnosticResult(r.Context(), endpointID, req.DiagnosticResult)
	s.persistFirewallAuditTelemetry(r.Context(), endpointID, req.FirewallAudit)

	_, cronsDigest, cronsOK, cronsErr := resolveCronsArtifact(r.Context(), s.cfg.ArtifactStore, s.cfg.ConfigRepoPath, ep.Fleet, endpointID, releaseRef)
	if cronsErr != nil {
		slog.Warn("resolve crons artifact", "endpoint", endpointID, "err", cronsErr)
	}
	if req.CronsDigest != "" {
		cronsDigest = req.CronsDigest
	}
	if cronsOK {
		s.persistCronResults(r.Context(), endpointID, releaseRef, cronsDigest, req)
	}
	dueCrons, activeCronsDigest := s.dueCronsForEndpoint(r.Context(), endpointID, ep.Fleet, req.Labels)
	if activeCronsDigest != "" {
		cronsDigest = activeCronsDigest
	}

	policy := s.remediationPolicy(r.Context(), ep.Fleet)
	if s.cfg.Admin != nil {
		if fresh, ok, err := s.cfg.Admin.GetEndpoint(endpointID); err == nil && ok {
			ep = fresh
		}
	}
	upgrade := s.agentUpgradeInstruction(ep)
	diagnostic := s.diagnosticCollectionForEndpoint(r.Context(), endpointID)
	executionLeases, err := s.executionLeases(endpointID, req.ChangePreflights)
	if err != nil {
		w.Header().Set("Retry-After", "5")
		http.Error(w, ErrChangeControlPersistenceUnavailable, http.StatusServiceUnavailable)
		return
	}

	if sync.Unchanged(req.LastDigest, digest, req.LastReleaseRef, releaseRef) {
		if err := s.recordCompatibleTarget(r.Context(), ep, releaseRef, digest, selectedSchemaVersion, false); err != nil {
			http.Error(w, "delivery state unavailable", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, syncResponse{
			Unchanged:            true,
			ReleaseRef:           releaseRef,
			Digest:               digest,
			RemediationPolicy:    policy,
			AgentUpgrade:         upgrade,
			DueCrons:             dueCrons,
			CronsDigest:          cronsDigest,
			DiagnosticCollection: diagnostic,
			ExecutionLeases:      executionLeases,
			RebootAcknowledged:   rebootAcknowledgement(req.RebootIntent),
			NetworkAcknowledged:  networkAcknowledgement(req.NetworkIntent),
		})
		return
	}
	if err := s.recordCompatibleTarget(r.Context(), ep, releaseRef, digest, selectedSchemaVersion, true); err != nil {
		http.Error(w, "delivery state unavailable", http.StatusServiceUnavailable)
		return
	}

	writeJSON(w, syncResponse{
		ReleaseRef:           releaseRef,
		Digest:               digest,
		ArtifactYAML:         artifact,
		RemediationPolicy:    policy,
		AgentUpgrade:         upgrade,
		DueCrons:             dueCrons,
		CronsDigest:          cronsDigest,
		DiagnosticCollection: diagnostic,
		ExecutionLeases:      executionLeases,
		RebootAcknowledged:   rebootAcknowledgement(req.RebootIntent),
		NetworkAcknowledged:  networkAcknowledgement(req.NetworkIntent),
	})
}

func admitStateReport(req *syncRequest) error {
	if req == nil || req.Drift == nil || len(req.Drift.Report) == 0 {
		return nil
	}
	payload, err := registry.ParseStateReportPayload(req.Drift.Report)
	if err != nil {
		return err
	}
	req.stateReport = &payload
	return nil
}

func admitCapabilityDocument(request *syncRequest) error {
	if request == nil || len(request.CapabilityDocument) == 0 {
		return nil
	}
	document, err := capabilitydoc.Decode(request.CapabilityDocument)
	if err != nil {
		return err
	}
	if err := document.Validate(); err != nil {
		return err
	}
	if request.AgentVersion == "" || request.AgentVersion != document.AgentVersion {
		return errors.New("capability document agent version does not match sync metadata")
	}
	request.capabilityDocument = &document
	return nil
}

func (s *Server) persistCurrentCapabilityDocument(ctx context.Context, endpointID string, request syncRequest) error {
	if request.capabilityDocument == nil || s.cfg.CapabilityDocuments == nil {
		return nil
	}
	canonical, err := request.capabilityDocument.CanonicalBody()
	if err != nil {
		return err
	}
	record := registry.CapabilityDocumentRecord{
		EndpointID: endpointID, Digest: request.capabilityDocument.Digest,
		CanonicalDocument: canonical, ReceivedAt: s.cfg.Now().UTC(),
	}
	if _, err = s.cfg.CapabilityDocuments.StoreEndpointCapabilityDocument(ctx, record); err != nil {
		return err
	}
	s.capabilityMu.Lock()
	s.currentCapabilities[endpointID] = record
	s.capabilityMu.Unlock()
	return nil
}

func (s *Server) currentCapabilityEvidence(endpointID string) (registry.CapabilityDocumentRecord, bool) {
	s.capabilityMu.RLock()
	defer s.capabilityMu.RUnlock()
	record, ok := s.currentCapabilities[endpointID]
	record.CanonicalDocument = append([]byte(nil), record.CanonicalDocument...)
	return record, ok
}

func (s *Server) clearCurrentCapabilityEvidence(endpointID string) {
	s.capabilityMu.Lock()
	delete(s.currentCapabilities, endpointID)
	s.capabilityMu.Unlock()
}

func (s *Server) hasPersistedCapabilityDocument(ctx context.Context, endpointID string) (bool, error) {
	if s.cfg.CapabilityDocuments == nil {
		return false, nil
	}
	_, ok, err := s.cfg.CapabilityDocuments.GetEndpointCapabilityDocument(ctx, endpointID)
	return ok, err
}

func rebootAcknowledgement(intent *sync.RebootIntentPayload) string {
	if intent == nil {
		return ""
	}
	return intent.Generation
}

func networkAcknowledgement(intent *sync.NetworkIntentPayload) string {
	if intent == nil {
		return ""
	}
	return intent.ID
}

func (s *Server) executionLeases(endpointID string, preflights []changecontrol.PreflightReport) ([]changecontrol.ExecutionLease, error) {
	if s.cfg.ChangeControl == nil {
		return nil, nil
	}
	var leases []changecontrol.ExecutionLease
	for _, preflight := range preflights {
		preflight.EndpointID = endpointID
		lease, issued, err := s.cfg.ChangeControl.IssueExecutionLease(preflight.ChangeRequestID, preflight)
		if err != nil {
			if changecontrol.IsPersistenceError(err) {
				return nil, err
			}
			slog.Warn("issue execution lease", "change_request", preflight.ChangeRequestID, "endpoint", endpointID, "err", err)
			continue
		}
		if issued {
			leases = append(leases, lease)
		}
	}
	return leases, nil
}

func (s *Server) releaseRef(ctx context.Context) string {
	if s.cfg.ReleaseRefSrc != nil {
		if ref := s.cfg.ReleaseRefSrc.ReleaseRef(ctx); ref != "" {
			return ref
		}
	}
	return s.cfg.ReleaseRef
}

func (s *Server) remediationPolicy(ctx context.Context, fleet string) string {
	if s.cfg.FleetSettings == nil {
		return "auto"
	}
	policy, err := s.cfg.FleetSettings.RemediationPolicy(ctx, fleet)
	if err != nil {
		slog.Warn("remediation policy lookup", "fleet", fleet, "err", err)
		return "auto"
	}
	if policy == "" {
		return "auto"
	}
	return policy
}

func (s *Server) recordCheckIn(ctx context.Context, endpointID, releaseRef, digest string) {
	if s.cfg.Telemetry == nil {
		return
	}
	if err := s.cfg.Telemetry.RecordEndpointCheckIn(ctx, endpointID, releaseRef, digest); err != nil {
		slog.Warn("persist endpoint check-in", "endpoint", endpointID, "err", err)
	}
}

func (s *Server) endpointHasActiveArtifact(ctx context.Context, endpoint registry.Endpoint) bool {
	if endpoint.LastCheckIn != nil && strings.TrimSpace(endpoint.LastCheckIn.ReleaseRef) != "" && strings.TrimSpace(endpoint.LastCheckIn.Digest) != "" {
		return true
	}
	if s.cfg.DeliveryStates == nil {
		return false
	}
	state, ok, err := s.cfg.DeliveryStates.GetEndpointDeliveryState(ctx, endpoint.ID)
	return err == nil && ok && state.ActiveReleaseRef != "" && state.ActiveDigest != ""
}

func (s *Server) acknowledgeOfferedArtifact(ctx context.Context, endpointID string, req syncRequest) error {
	if s.cfg.DeliveryStates == nil {
		return nil
	}
	releaseRef, digest, ok := reportedActiveArtifact(req)
	if !ok {
		return nil
	}
	state, exists, err := s.cfg.DeliveryStates.GetEndpointDeliveryState(ctx, endpointID)
	if err != nil || !exists {
		return err
	}
	if state.OfferedReleaseRef != releaseRef || state.OfferedDigest != digest {
		return nil
	}
	state.ActiveReleaseRef = state.OfferedReleaseRef
	state.ActiveDigest = state.OfferedDigest
	state.ActiveSchemaVersion = state.OfferedSchemaVersion
	state.ActiveAt = s.cfg.Now().UTC()
	state.OfferedReleaseRef = ""
	state.OfferedDigest = ""
	state.OfferedSchemaVersion = 0
	state.OfferedAt = time.Time{}
	if err := s.cfg.DeliveryStates.StoreEndpointDeliveryState(ctx, state); err != nil {
		return err
	}
	s.recordCheckIn(ctx, endpointID, releaseRef, digest)
	return nil
}

func (s *Server) recordCompatibleTarget(ctx context.Context, endpoint registry.Endpoint, releaseRef, digest string, schemaVersion int, offered bool) error {
	if s.cfg.DeliveryStates == nil {
		return nil
	}
	state, _, err := s.cfg.DeliveryStates.GetEndpointDeliveryState(ctx, endpoint.ID)
	if err != nil {
		return err
	}
	state.EndpointID = endpoint.ID
	seedActiveDeliveryState(&state, endpoint)
	state.TargetReleaseRef = releaseRef
	state.CapabilityBlockedTargetRef = ""
	state.MissingRequirements = nil
	state.Unmanaged = false
	if offered {
		state.OfferedReleaseRef = releaseRef
		state.OfferedDigest = digest
		state.OfferedSchemaVersion = schemaVersion
		state.OfferedAt = s.cfg.Now().UTC()
	}
	return s.cfg.DeliveryStates.StoreEndpointDeliveryState(ctx, state)
}

func (s *Server) recordCapabilityBlock(ctx context.Context, endpoint registry.Endpoint, targetReleaseRef string, missing []sync.MissingRequirement, unmanaged bool) error {
	if s.cfg.DeliveryStates == nil {
		return nil
	}
	state, _, err := s.cfg.DeliveryStates.GetEndpointDeliveryState(ctx, endpoint.ID)
	if err != nil {
		return err
	}
	state.EndpointID = endpoint.ID
	seedActiveDeliveryState(&state, endpoint)
	state.TargetReleaseRef = targetReleaseRef
	state.CapabilityBlockedTargetRef = targetReleaseRef
	state.MissingRequirements = make([]registry.MissingRequirement, 0, len(missing))
	for _, requirement := range missing {
		state.MissingRequirements = append(state.MissingRequirements, registry.MissingRequirement{ID: requirement.ID, Revision: requirement.Revision})
	}
	state.Unmanaged = unmanaged
	return s.cfg.DeliveryStates.StoreEndpointDeliveryState(ctx, state)
}

func seedActiveDeliveryState(state *registry.EndpointDeliveryState, endpoint registry.Endpoint) {
	if state.ActiveDigest != "" || endpoint.LastCheckIn == nil {
		return
	}
	state.ActiveReleaseRef = endpoint.LastCheckIn.ReleaseRef
	state.ActiveDigest = endpoint.LastCheckIn.Digest
	state.ActiveAt = endpoint.LastCheckIn.At
}

func reportedActiveArtifact(req syncRequest) (string, string, bool) {
	releaseRef := strings.TrimSpace(req.LastReleaseRef)
	digest := strings.TrimSpace(req.LastDigest)
	if releaseRef == "" || digest == "" || releaseRef != req.LastReleaseRef || digest != req.LastDigest || len(releaseRef) > 512 || len(digest) > 512 {
		return "", "", false
	}
	return releaseRef, digest, true
}

func (s *Server) persistTelemetry(ctx context.Context, endpointID, releaseRef string, req syncRequest) {
	if s.cfg.Telemetry == nil {
		return
	}
	if len(req.Labels) > 0 {
		if err := s.cfg.Telemetry.UpsertEndpointLabels(ctx, endpointID, req.Labels); err != nil {
			slog.Warn("persist endpoint labels", "endpoint", endpointID, "err", err)
		}
	}
	if len(req.Usernames) > 0 {
		if err := s.cfg.Telemetry.UpdateEndpointUsernames(ctx, endpointID, req.Usernames); err != nil {
			slog.Warn("persist endpoint usernames", "endpoint", endpointID, "err", err)
		}
	}
	if req.SystemInfo != nil && len(req.SystemInfo.Report) > 0 {
		if err := s.cfg.Telemetry.UpsertEndpointSystemInfo(ctx, endpointID, req.SystemInfo.Digest, req.SystemInfo.Report); err != nil {
			slog.Warn("persist endpoint system info", "endpoint", endpointID, "err", err)
		}
	}
	if req.Drift != nil && req.stateReport != nil {
		reportedReleaseRef := releaseRef
		if req.LastReleaseRef != "" && req.LastReleaseRef == strings.TrimSpace(req.LastReleaseRef) && len(req.LastReleaseRef) <= 512 {
			reportedReleaseRef = req.LastReleaseRef
		}
		digest := req.Drift.Digest
		if digest == "" {
			digest = req.LastDigest
		}
		if err := s.cfg.Telemetry.InsertDriftReport(ctx, endpointID, reportedReleaseRef, digest, *req.stateReport); err != nil {
			slog.Warn("persist drift report", "endpoint", endpointID, "err", err)
		}
	}
	if req.ApplyFailure != nil && req.ApplyFailure.ResourceAddress != "" {
		reportedReleaseRef := releaseRef
		if req.LastReleaseRef != "" && req.LastReleaseRef == strings.TrimSpace(req.LastReleaseRef) && len(req.LastReleaseRef) <= 512 {
			reportedReleaseRef = req.LastReleaseRef
		}
		failure := executor.NewSafeError("apply_failed", "legacy_provider_apply", errors.New(req.ApplyFailure.Message))
		if req.ApplyFailure.Failure != nil {
			failure = *req.ApplyFailure.Failure
		}
		if err := s.cfg.Telemetry.InsertApplyFailure(
			ctx,
			endpointID,
			reportedReleaseRef,
			req.ApplyFailure.ResourceAddress,
			failure,
		); err != nil {
			slog.Warn("persist apply failure", "endpoint", endpointID, "err", err)
		}
	}
}

func (s *Server) persistFirewallAuditTelemetry(ctx context.Context, endpointID string, payload *firewallAuditPayload) {
	if s.cfg.Telemetry == nil || payload == nil || len(payload.Report) == 0 {
		return
	}
	if err := s.cfg.Telemetry.InsertFirewallAuditReport(ctx, endpointID, payload.Digest, payload.Report); err != nil {
		slog.Warn("persist firewall audit", "endpoint", endpointID, "err", err)
	}
}

func endpointIDFromRequest(r *http.Request) (string, error) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return "", errNoClientCert
	}
	return identity.EndpointIDFromCert(r.TLS.PeerCertificates[0])
}

var errNoClientCert = errString("no client certificate")

type errString string

func (e errString) Error() string { return string(e) }

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	_ = enc.Encode(v)
}

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !acceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		gzw := &gzipResponseWriter{Writer: gz, ResponseWriter: w}
		next.ServeHTTP(gzw, r)
	})
}

func acceptsGzip(r *http.Request) bool {
	for _, v := range r.Header.Values("Accept-Encoding") {
		if strings.Contains(v, "gzip") {
			return true
		}
	}
	return false
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
	wroteHeader bool
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.Writer.Write(b)
}
