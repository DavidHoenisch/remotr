package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	gosysinfo "github.com/DavidHoenisch/go-sysinfo"
	"github.com/DavidHoenisch/remotr/internal/agent/credentials"
	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/inventory"
	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
	"github.com/DavidHoenisch/remotr/internal/agent/pipeline"
	"github.com/DavidHoenisch/remotr/internal/agent/rebootstate"
	"github.com/DavidHoenisch/remotr/internal/agent/sync"
	"github.com/DavidHoenisch/remotr/internal/agent/upgrade"
	"github.com/DavidHoenisch/remotr/internal/apppackages"
	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
	"github.com/DavidHoenisch/remotr/internal/documenthash"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

// syncRunState tracks the last artifact the agent successfully processed.
type syncRunState struct {
	lastDigest             string
	lastReleaseRef         string
	lastArtifactYAML       []byte
	throttler              *inventory.Throttler
	stateDir               string
	pkgURLs                apppackages.URLResolver
	serverURL              string
	tlsCfg                 *tls.Config
	rebootState            *rebootstate.Store
	networkState           *networkstate.Store
	rebootRunner           executil.Runner
	now                    func() time.Time
	bootID                 func() (string, error)
	secretResolver         secrets.Resolver
	secretCache            *secrets.AuthorityCachingResolver
	capabilityGenerator    *capabilitydoc.Generator
	readCapabilityFacts    func() (facts.Facts, error)
	acceptedDocumentHashes map[string]string
	lastComplianceHash     string
	lastFirewallAuditHash  string
}

func newSyncRunState(stateDir, serverURL string, tlsCfg *tls.Config, pkgURLs apppackages.URLResolver) syncRunState {
	interval := envDurationOr("REMOTR_SYSTEM_INFO_INTERVAL", time.Hour)
	th := inventory.NewThrottler(interval, 5*time.Minute)
	var acceptedDocumentHashes map[string]string
	if stateDir != "" {
		if st, err := credentials.LoadState(stateDir); err == nil {
			th.LoadState(inventory.ThrottleState{
				LastSentAt:     st.SystemInfoSentAt,
				LastSentDigest: st.SystemInfoSentDigest,
			})
			candidate := documenthash.Summary{Version: documenthash.CurrentVersion, Documents: st.AcceptedDocumentHashes}
			if len(st.AcceptedDocumentHashes) > 0 && candidate.Validate() == nil {
				acceptedDocumentHashes = cloneDocumentHashes(st.AcceptedDocumentHashes)
			}
		}
	}
	capabilityGenerator, capabilityErr := capabilitydoc.NewDefaultGenerator([]int{0, 1})
	if capabilityErr != nil {
		slog.Error("initialize capability document generator", "err", capabilityErr)
	}
	state := syncRunState{
		throttler:              th,
		stateDir:               stateDir,
		pkgURLs:                pkgURLs,
		serverURL:              serverURL,
		tlsCfg:                 tlsCfg,
		rebootState:            rebootstate.New(stateDir),
		rebootRunner:           executil.OSRunner{},
		now:                    time.Now,
		bootID:                 readBootID,
		capabilityGenerator:    capabilityGenerator,
		readCapabilityFacts:    facts.Read,
		acceptedDocumentHashes: acceptedDocumentHashes,
	}
	secretHTTPClient := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}, Timeout: 30 * time.Second}
	state.secretCache = secrets.NewAuthorityCachingResolver(
		secrets.NewRemotrProvider(serverURL, secretHTTPClient),
		secrets.AuthorityCacheOptions{},
	)
	state.secretResolver = secrets.NewRoutingResolver(
		secrets.NewLocalFileProvider(),
		state.secretCache,
	)
	if network, err := networkstate.New(networkstate.Options{Root: stateDir, Runner: executil.OSRunner{}}); err == nil {
		state.networkState = network
	} else {
		slog.Error("initialize network transaction state", "err", err)
	}
	return state
}

func cloneDocumentHashes(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	for name, hash := range input {
		output[name] = hash
	}
	return output
}

func complianceReportHash(report *sync.DriftPayload) string {
	if report == nil {
		return ""
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(report.Digest))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(report.Report)
	return fmt.Sprintf("%x", digest.Sum(nil))
}

func firewallAuditHash(report *sync.FirewallAuditPayload) string {
	if report == nil {
		return ""
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(report.Digest))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(report.Report)
	return fmt.Sprintf("%x", digest.Sum(nil))
}

func (s *syncRunState) attachRepeatableDocuments(request sync.Request, capability *capabilitydoc.Document) (sync.Request, error) {
	canonicalCapability, err := capability.CanonicalBody()
	if err != nil {
		return sync.Request{}, err
	}
	capabilityHash, err := documenthash.Digest(documenthash.Capability, canonicalCapability)
	if err != nil {
		return sync.Request{}, err
	}
	deliveryDocument, err := documenthash.CanonicalDelivery(request.LastReleaseRef, request.LastDigest)
	if err != nil {
		return sync.Request{}, err
	}
	deliveryHash, err := documenthash.Digest(documenthash.Delivery, deliveryDocument)
	if err != nil {
		return sync.Request{}, err
	}
	if len(request.Labels) == 0 {
		endpointFacts, err := s.readCapabilityFacts()
		if err != nil {
			return sync.Request{}, err
		}
		request.Labels = map[string]string{"distro": string(endpointFacts.Distro), "arch": string(endpointFacts.Arch)}
	}
	targetingDocument, err := documenthash.CanonicalTargeting(request.Labels, request.Usernames)
	if err != nil {
		return sync.Request{}, err
	}
	targetingHash, err := documenthash.Digest(documenthash.Targeting, targetingDocument)
	if err != nil {
		return sync.Request{}, err
	}
	hashes := map[string]string{
		documenthash.Capability: capabilityHash,
		documenthash.Delivery:   deliveryHash,
		documenthash.Targeting:  targetingHash,
	}
	if s.acceptedDocumentHashes[documenthash.Capability] != capabilityHash {
		request.CapabilityDocument = capability
	}

	if request.SystemInfo != nil {
		canonicalSystemInfo, err := documenthash.CanonicalJSON(request.SystemInfo.Report)
		if err != nil {
			return sync.Request{}, err
		}
		systemInfoHash, err := documenthash.Digest(documenthash.SystemInformation, canonicalSystemInfo)
		if err != nil {
			return sync.Request{}, err
		}
		hashes[documenthash.SystemInformation] = systemInfoHash
		if s.acceptedDocumentHashes[documenthash.SystemInformation] == systemInfoHash {
			request.SystemInfo = nil
		}
	} else if hash := s.acceptedDocumentHashes[documenthash.SystemInformation]; hash != "" {
		hashes[documenthash.SystemInformation] = hash
	}
	if documenthash.Equal(s.acceptedDocumentHashes[documenthash.Targeting], targetingHash) {
		request.Labels = nil
		request.Usernames = nil
	}
	request.DocumentHashes = &documenthash.Summary{Version: documenthash.CurrentVersion, Documents: hashes}
	return request, nil
}

func (s *syncRunState) acceptDocumentHashes(request sync.Request, response sync.Response) error {
	next := cloneDocumentHashes(s.acceptedDocumentHashes)
	if next == nil {
		next = make(map[string]string)
	}
	changed := false
	if response.AcceptedDocumentHashes != nil {
		if err := response.AcceptedDocumentHashes.Validate(); err != nil {
			return fmt.Errorf("invalid accepted document hashes: %w", err)
		}
		for name, hash := range response.AcceptedDocumentHashes.Documents {
			if request.DocumentHashes == nil || !documenthash.Equal(request.DocumentHashes.Documents[name], hash) {
				return fmt.Errorf("server acknowledged an unsubmitted document hash")
			}
			if next[name] != hash {
				next[name] = hash
				changed = true
			}
		}
	}
	if len(response.RequestedDocuments) > documenthash.MaxDocuments {
		return fmt.Errorf("server requested too many documents")
	}
	requested := make(map[string]bool, len(response.RequestedDocuments))
	for _, name := range response.RequestedDocuments {
		if !documenthash.Known(name) || requested[name] {
			return fmt.Errorf("server requested an invalid document")
		}
		requested[name] = true
		if _, ok := next[name]; ok {
			delete(next, name)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if s.stateDir != "" {
		state, err := credentials.LoadState(s.stateDir)
		if err != nil {
			return fmt.Errorf("load accepted document state: %w", err)
		}
		state.AcceptedDocumentHashes = cloneDocumentHashes(next)
		if err := credentials.SaveState(s.stateDir, state); err != nil {
			return fmt.Errorf("persist accepted document state: %w", err)
		}
	}
	s.acceptedDocumentHashes = next
	return nil
}

func (s *syncRunState) documentAcknowledged(request sync.Request, name string) bool {
	if request.DocumentHashes == nil {
		return false
	}
	return documenthash.Equal(s.acceptedDocumentHashes[name], request.DocumentHashes.Documents[name])
}

func (s *syncRunState) currentCapabilityDocument(agentVersion string) (*capabilitydoc.Document, error) {
	if s.capabilityGenerator == nil || s.readCapabilityFacts == nil {
		return nil, fmt.Errorf("capability document generator is unavailable")
	}
	endpointFacts, err := s.readCapabilityFacts()
	if err != nil {
		return nil, fmt.Errorf("read capability facts: %w", err)
	}
	document, err := s.capabilityGenerator.Generate(endpointFacts, agentVersion)
	if err != nil {
		return nil, err
	}
	if err := document.Validate(); err != nil {
		return nil, err
	}
	return &document, nil
}

func readBootID() (string, error) {
	raw, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", fmt.Errorf("read boot identity: %w", err)
	}
	bootID := strings.TrimSpace(string(raw))
	if bootID == "" {
		return "", fmt.Errorf("read boot identity: empty value")
	}
	return bootID, nil
}

func (s *syncRunState) recordRebootRequirement(pending *sync.Pending, applied engine.ApplyResult) error {
	var sources []rebootstate.Source
	for _, item := range applied.Items {
		if item.RebootRequired == executor.RebootRequired && (item.Status == executor.Changed || item.Status == executor.NoChange) {
			sources = append(sources, rebootstate.Source{Address: item.Address, Name: item.Name, Provider: item.Provider})
		}
	}
	status, err := s.rebootState.Record(sources)
	if err != nil {
		return err
	}
	pending.SetRebootRequired(status)
	return nil
}

func (s *syncRunState) executeAcknowledgedReboot(intent *sync.RebootIntentPayload) error {
	if intent == nil {
		return nil
	}
	bootID, err := s.bootID()
	if err != nil {
		return err
	}
	attempt, err := s.rebootState.Acknowledge(intent.Generation, s.now().UTC(), bootID)
	if err != nil {
		return err
	}
	_, _, commandErr := s.rebootRunner.Run("systemctl", "reboot")
	if commandErr == nil {
		return nil
	}
	if _, stateErr := s.rebootState.MarkAttemptFailed(attempt.Generation, "reboot_command_failed"); stateErr != nil {
		return fmt.Errorf("reboot command failed and state update failed: %v", stateErr)
	}
	return fmt.Errorf("reboot command failed; command output was redacted")
}

func (s *syncRunState) refreshRebootCoordination(pending *sync.Pending) error {
	bootID, err := s.bootID()
	if err != nil {
		return err
	}
	now := s.now().UTC()
	status, err := s.rebootState.Reconcile(bootID, now)
	if err != nil {
		return err
	}
	pending.SetRebootRequired(status)
	pending.SetRebootIntent(nil)
	if status.Intent == nil || status.Intent.Phase != rebootstate.PhaseAwaitingAcknowledgement || now.Before(status.Intent.NotBefore) {
		return nil
	}
	if !status.Intent.Deadline.IsZero() && !now.Before(status.Intent.Deadline) {
		return nil
	}
	pending.SetRebootIntent(status.Intent)
	return nil
}

func (s *syncRunState) refreshNetworkCoordination(pending *sync.Pending) error {
	if s.networkState == nil {
		pending.SetNetworkIntent(nil)
		return nil
	}
	status, err := s.networkState.Reconcile(context.Background())
	if err != nil {
		return err
	}
	pending.SetNetworkIntent(nil)
	if status.Intent != nil && status.Intent.Phase == networkstate.PhaseAwaitingAcknowledgement {
		pending.SetNetworkIntent(status.Intent)
	}
	return nil
}

func (s *syncRunState) applyConfig(
	ctx context.Context,
	resp sync.Response,
	pending *sync.Pending,
) {
	if len(resp.ArtifactYAML) == 0 {
		return
	}
	slog.Info("sync received artifact",
		"releaseRef", resp.ReleaseRef,
		"digest", resp.Digest,
		"bytes", len(resp.ArtifactYAML),
	)
	policy := pipeline.PolicyFromResponse(resp.RemediationPolicy)
	result, err := pipeline.Run(ctx, resp.ArtifactYAML, policy, nil, s.pkgURLs, s.serverURL,
		engine.WithStateDir(s.stateDir), engine.WithSecretResolver(s.secretResolver), engine.WithArtifactDigest(resp.Digest),
		engine.WithExecutionLeases(resp.ExecutionLeases))
	if stateErr := s.recordRebootRequirement(pending, result.Apply); stateErr != nil {
		slog.Error("persist reboot-required state", "err", stateErr)
	}
	if stateErr := s.refreshRebootCoordination(pending); stateErr != nil {
		slog.Error("refresh reboot coordination", "err", stateErr)
	}
	if stateErr := s.refreshNetworkCoordination(pending); stateErr != nil {
		slog.Error("refresh network transaction state", "err", stateErr)
	}
	pending.SetFromPipeline(result.Labels, result.Drift, result.Apply, result.ApplyFailure, resp.Digest)
	if err != nil {
		slog.Error("pipeline failed; artifact will be retried", "err", err)
		if result.ApplyFailure != nil {
			slog.Info("reporting apply failure on next sync", "address", result.ApplyFailure.Address)
		}
		return
	}
	s.lastArtifactYAML = append([]byte(nil), resp.ArtifactYAML...)
	if resp.Digest != "" {
		s.lastDigest = resp.Digest
	}
	if resp.ReleaseRef != "" {
		s.lastReleaseRef = resp.ReleaseRef
	}
}

func (s *syncRunState) prepareComplianceReport(
	ctx context.Context,
	pending *sync.Pending,
) {
	if len(s.lastArtifactYAML) == 0 {
		return
	}
	result, err := pipeline.Check(ctx, s.lastArtifactYAML, nil, s.pkgURLs, s.serverURL,
		engine.WithStateDir(s.stateDir), engine.WithSecretResolver(s.secretResolver), engine.WithArtifactDigest(s.lastDigest))
	if err != nil {
		slog.Error("compliance check failed", "err", err)
		return
	}
	if stateErr := s.recordRebootRequirement(pending, result.Apply); stateErr != nil {
		slog.Error("load reboot-required state", "err", stateErr)
	}
	if stateErr := s.refreshRebootCoordination(pending); stateErr != nil {
		slog.Error("refresh reboot coordination", "err", stateErr)
	}
	pending.SetFromPipeline(result.Labels, result.Drift, result.Apply, nil, s.lastDigest)
	if complianceReportHash(pending.Drift) == s.lastComplianceHash {
		pending.Drift = nil
	}
}

func (s *syncRunState) prepareSystemInfo(pending *sync.Pending) {
	if s.throttler == nil {
		return
	}
	now := time.Now().UTC()
	snap := inventory.Collect(gosysinfo.Reader{})
	digest, err := inventory.Digest(snap)
	if err != nil {
		slog.Error("system info digest", "err", err)
		return
	}
	if !s.throttler.ShouldReport(now, digest) {
		return
	}
	raw, err := inventory.MarshalJSON(snap)
	if err != nil {
		slog.Error("system info marshal", "err", err)
		return
	}
	pending.SetSystemInfo(digest, json.RawMessage(raw))
}

func (s *syncRunState) persistSystemInfoSent(sent sync.Request) {
	if sent.SystemInfo == nil || s.throttler == nil {
		return
	}
	now := time.Now().UTC()
	s.throttler.MarkSent(now, sent.SystemInfo.Digest)
	if s.stateDir == "" {
		return
	}
	st, err := credentials.LoadState(s.stateDir)
	if err != nil {
		slog.Warn("load agent state for system info", "err", err)
		return
	}
	st.SystemInfoSentAt = now
	st.SystemInfoSentDigest = sent.SystemInfo.Digest
	if err := credentials.SaveState(s.stateDir, st); err != nil {
		slog.Warn("persist system info throttle state", "err", err)
	}
}

func (s *syncRunState) prepareFirewallAudit(pending *sync.Pending) {
	const auditPath = "/var/log/remotr/firewall-audit.log"
	data, err := os.ReadFile(auditPath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("read firewall audit log", "err", err)
		}
		return
	}
	if len(data) == 0 {
		return
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	pending.SetFirewallAudit(digest, json.RawMessage(data))
}

func (s *syncRunState) elideRepeatedFirewallAudit(pending *sync.Pending) {
	if firewallAuditHash(pending.FirewallAudit) == s.lastFirewallAuditHash {
		pending.FirewallAudit = nil
	}
}

func (s *syncRunState) maybeUpgrade(
	resp sync.Response,
	pending *sync.Pending,
	currentVersion string,
) bool {
	if resp.AgentUpgrade == nil {
		return false
	}
	inst := upgrade.Instruction{
		Version:    resp.AgentUpgrade.Version,
		GitHubRepo: resp.AgentUpgrade.GitHubRepo,
	}
	if !upgrade.Needed(inst, currentVersion) {
		return false
	}
	slog.Info("agent upgrade requested", "version", inst.Version)
	pending.SetAgentUpgradeStatus(inst.Version, "installing", "")
	if err := upgrade.Apply(inst, upgrade.Options{
		CurrentVersion: currentVersion,
		BinDir:         envOr("REMOTR_BIN_DIR", "/usr/local/bin"),
	}); err != nil {
		slog.Error("agent upgrade failed", "err", err)
		pending.SetAgentUpgradeStatus(inst.Version, "failed", err.Error())
		return true
	}
	return true
}

func (s *syncRunState) runOnce(
	ctx context.Context,
	client *sync.Client,
	pending *sync.Pending,
	currentVersion string,
) error {
	if err := s.refreshRebootCoordination(pending); err != nil {
		slog.Error("refresh reboot coordination", "err", err)
	}
	if err := s.refreshNetworkCoordination(pending); err != nil {
		slog.Error("refresh network transaction state", "err", err)
	}
	s.prepareSystemInfo(pending)
	s.prepareFirewallAudit(pending)
	s.elideRepeatedFirewallAudit(pending)
	s.prepareComplianceReport(ctx, pending)
	req := pending.Request(s.lastDigest, s.lastReleaseRef, currentVersion)
	if usernames, err := interactiveuser.ListUsernames(); err == nil && len(usernames) > 0 {
		req.Usernames = usernames
	}
	capabilityDocument, err := s.currentCapabilityDocument(currentVersion)
	if err != nil {
		return fmt.Errorf("generate capability document: %w", err)
	}
	req, err = s.attachRepeatableDocuments(req, capabilityDocument)
	if err != nil {
		return fmt.Errorf("prepare repeatable Sync documents: %w", err)
	}
	resp, err := client.Sync(req)
	if err != nil {
		slog.Error("sync failed", "err", err)
		return err
	}
	if err := s.acceptDocumentHashes(req, resp); err != nil {
		return err
	}
	if s.secretCache != nil {
		s.secretCache.SetAuthorityToken(resp.SecretAuthorityToken)
	}
	if req.Drift != nil {
		s.lastComplianceHash = complianceReportHash(req.Drift)
	}
	if req.FirewallAudit != nil {
		s.lastFirewallAuditHash = firewallAuditHash(req.FirewallAudit)
	}
	if req.SystemInfo != nil && s.documentAcknowledged(req, documenthash.SystemInformation) {
		s.persistSystemInfoSent(req)
	}
	if acknowledged := acknowledgedRebootIntent(req, resp); acknowledged != nil {
		if err := s.executeAcknowledgedReboot(acknowledged); err != nil {
			slog.Error("execute acknowledged reboot", "generation", acknowledged.Generation, "err", err)
		}
	}
	if acknowledged := acknowledgedNetworkIntent(req, resp); acknowledged != nil && s.networkState != nil {
		if _, err := s.networkState.Acknowledge(ctx, acknowledged.ID); err != nil {
			slog.Error("acknowledge network transaction", "transaction", acknowledged.ID, "err", err)
		}
	}
	clearable := req
	if !s.documentAcknowledged(req, documenthash.SystemInformation) {
		clearable.SystemInfo = nil
	}
	pending.ClearSent(clearable)

	if len(resp.ArtifactYAML) > 0 {
		s.applyConfig(ctx, resp, pending)
	} else if len(resp.ExecutionLeases) > 0 && len(s.lastArtifactYAML) > 0 && resp.Unchanged && sync.Unchanged(s.lastDigest, resp.Digest, s.lastReleaseRef, resp.ReleaseRef) {
		retry := resp
		retry.ArtifactYAML = append([]byte(nil), s.lastArtifactYAML...)
		s.applyConfig(ctx, retry, pending)
	} else if resp.CapabilityBlocked != nil {
		slog.Info("sync capability blocked",
			"target_release_ref", resp.CapabilityBlocked.TargetReleaseRef,
			"missing_requirements", len(resp.CapabilityBlocked.MissingRequirements),
			"unmanaged", resp.CapabilityBlocked.Unmanaged,
		)
	} else if sync.Unchanged(s.lastDigest, resp.Digest, s.lastReleaseRef, resp.ReleaseRef) {
		slog.Info("sync unchanged", "digest", resp.Digest, "releaseRef", resp.ReleaseRef)
	} else {
		slog.Warn("sync response missing artifact", "digest", resp.Digest, "releaseRef", resp.ReleaseRef)
	}
	s.runDueCrons(ctx, resp, pending)
	if s.maybeUpgrade(resp, pending, currentVersion) {
		return nil
	}
	s.runDiagnosticCollection(ctx, resp, pending, currentVersion, s.serverURL, s.tlsCfg)
	return nil
}

func acknowledgedRebootIntent(request sync.Request, response sync.Response) *sync.RebootIntentPayload {
	if request.RebootIntent == nil || response.RebootAcknowledged == "" || response.RebootAcknowledged != request.RebootIntent.Generation {
		return nil
	}
	return request.RebootIntent
}

func acknowledgedNetworkIntent(request sync.Request, response sync.Response) *sync.NetworkIntentPayload {
	if request.NetworkIntent == nil || response.NetworkAcknowledged == "" || response.NetworkAcknowledged != request.NetworkIntent.ID {
		return nil
	}
	return request.NetworkIntent
}
