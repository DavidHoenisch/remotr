package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	gosysinfo "github.com/DavidHoenisch/go-sysinfo"
	"github.com/DavidHoenisch/remotr/internal/agent/credentials"
	"github.com/DavidHoenisch/remotr/internal/agent/inventory"
	"github.com/DavidHoenisch/remotr/internal/agent/pipeline"
	"github.com/DavidHoenisch/remotr/internal/agent/sync"
	"github.com/DavidHoenisch/remotr/internal/agent/upgrade"
)

// syncRunState tracks the last artifact the agent successfully processed.
type syncRunState struct {
	lastDigest       string
	lastReleaseRef   string
	lastArtifactYAML []byte
	throttler        *inventory.Throttler
	stateDir         string
}

func newSyncRunState(stateDir string) syncRunState {
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
	return syncRunState{
		throttler: th,
		stateDir:  stateDir,
	}
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
	result, err := pipeline.Run(ctx, resp.ArtifactYAML, policy, nil)
	pending.SetFromPipeline(result.Labels, result.Drift, result.ApplyFailure, resp.Digest)
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
	result, err := pipeline.Check(ctx, s.lastArtifactYAML, nil)
	if err != nil {
		slog.Error("compliance check failed", "err", err)
		return
	}
	pending.SetFromPipeline(result.Labels, result.Drift, nil, s.lastDigest)
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
) {
	s.prepareSystemInfo(pending)
	s.prepareComplianceReport(ctx, pending)
	req := pending.Request(s.lastDigest, s.lastReleaseRef, currentVersion)
	resp, err := client.Sync(req)
	if err != nil {
		slog.Error("sync failed", "err", err)
		return
	}
	s.persistSystemInfoSent(req)
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
		return
	}
}
