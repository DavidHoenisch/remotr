package main

import (
	"context"
	"log/slog"

	"github.com/DavidHoenisch/remotr/internal/agent/pipeline"
	"github.com/DavidHoenisch/remotr/internal/agent/sync"
	"github.com/DavidHoenisch/remotr/internal/agent/upgrade"
)

// syncRunState tracks the last artifact the agent successfully processed.
type syncRunState struct {
	lastDigest       string
	lastReleaseRef   string
	lastArtifactYAML []byte
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
	s.prepareComplianceReport(ctx, pending)
	req := pending.Request(s.lastDigest, s.lastReleaseRef, currentVersion)
	resp, err := client.Sync(req)
	if err != nil {
		slog.Error("sync failed", "err", err)
		return
	}
	pending.ClearSent(req)

	if len(resp.ArtifactYAML) > 0 {
		s.applyConfig(ctx, resp, pending)
	} else if sync.Unchanged(s.lastDigest, resp.Digest, s.lastReleaseRef, resp.ReleaseRef) {
		slog.Info("sync unchanged", "digest", resp.Digest, "releaseRef", resp.ReleaseRef)
	} else {
		slog.Warn("sync response missing artifact", "digest", resp.Digest, "releaseRef", resp.ReleaseRef)
	}
	if s.maybeUpgrade(resp, pending, currentVersion) {
		return
	}
}
