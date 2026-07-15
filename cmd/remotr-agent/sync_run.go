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
	"github.com/DavidHoenisch/remotr/internal/agent/inventory"
	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
	"github.com/DavidHoenisch/remotr/internal/agent/pipeline"
	"github.com/DavidHoenisch/remotr/internal/agent/rebootstate"
	"github.com/DavidHoenisch/remotr/internal/agent/sync"
	"github.com/DavidHoenisch/remotr/internal/agent/upgrade"
	"github.com/DavidHoenisch/remotr/internal/apppackages"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

// syncRunState tracks the last artifact the agent successfully processed.
type syncRunState struct {
	lastDigest       string
	lastReleaseRef   string
	lastArtifactYAML []byte
	throttler        *inventory.Throttler
	stateDir         string
	pkgURLs          apppackages.URLResolver
	serverURL        string
	tlsCfg           *tls.Config
	rebootState      *rebootstate.Store
	networkState     *networkstate.Store
	rebootRunner     executil.Runner
	now              func() time.Time
	bootID           func() (string, error)
	secretResolver   secrets.Resolver
}

func newSyncRunState(stateDir, serverURL string, tlsCfg *tls.Config, pkgURLs apppackages.URLResolver) syncRunState {
	interval := envDurationOr("REMOTR_SYSTEM_INFO_INTERVAL", time.Hour)
	th := inventory.NewThrottler(interval, 5*time.Minute)
	if stateDir != "" {
		if st, err := credentials.LoadState(stateDir); err == nil {
			th.LoadState(inventory.ThrottleState{
				LastSentAt:     st.SystemInfoSentAt,
				LastSentDigest: st.SystemInfoSentDigest,
			})
		}
	}
	state := syncRunState{
		throttler:    th,
		stateDir:     stateDir,
		pkgURLs:      pkgURLs,
		serverURL:    serverURL,
		tlsCfg:       tlsCfg,
		rebootState:  rebootstate.New(stateDir),
		rebootRunner: executil.OSRunner{},
		now:          time.Now,
		bootID:       readBootID,
	}
	secretHTTPClient := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}, Timeout: 30 * time.Second}
	state.secretResolver = secrets.NewRoutingResolver(
		secrets.NewLocalFileProvider(),
		secrets.NewRemotrProvider(serverURL, secretHTTPClient),
	)
	if network, err := networkstate.New(networkstate.Options{Root: stateDir, Runner: executil.OSRunner{}}); err == nil {
		state.networkState = network
	} else {
		slog.Error("initialize network transaction state", "err", err)
	}
	return state
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
	s.lastArtifactYAML = append([]byte(nil), resp.ArtifactYAML...)
	policy := pipeline.PolicyFromResponse(resp.RemediationPolicy)
	result, err := pipeline.Run(ctx, resp.ArtifactYAML, policy, nil, s.pkgURLs, s.serverURL,
		engine.WithStateDir(s.stateDir), engine.WithSecretResolver(s.secretResolver), engine.WithArtifactDigest(resp.Digest))
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
	if resp.Digest != "" {
		s.lastDigest = resp.Digest
	}
	if resp.ReleaseRef != "" {
		s.lastReleaseRef = resp.ReleaseRef
	}
	if err != nil {
		slog.Error("pipeline failed", "err", err)
		if result.ApplyFailure != nil {
			slog.Info("reporting apply failure on next sync", "address", result.ApplyFailure.Address)
		}
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
	s.prepareComplianceReport(ctx, pending)
	req := pending.Request(s.lastDigest, s.lastReleaseRef, currentVersion)
	if usernames, err := interactiveuser.ListUsernames(); err == nil && len(usernames) > 0 {
		req.Usernames = usernames
	}
	resp, err := client.Sync(req)
	if err != nil {
		slog.Error("sync failed", "err", err)
		return err
	}
	s.persistSystemInfoSent(req)
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
	pending.ClearSent(req)

	if len(resp.ArtifactYAML) > 0 {
		s.applyConfig(ctx, resp, pending)
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
