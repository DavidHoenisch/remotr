package server

import (
	"context"
	"log/slog"

	"github.com/DavidHoenisch/remotr/internal/agentversion"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

type agentUpgradePayload struct {
	Version     string `json:"version"`
	GitHubRepo  string `json:"githubRepo,omitempty"`
}

type agentUpgradeStatusPayload struct {
	Desired string `json:"desired,omitempty"`
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
}

func (s *Server) agentUpgradeInstruction(ep registry.Endpoint) *agentUpgradePayload {
	desired := ep.DesiredAgentVersion
	if desired == "" {
		return nil
	}
	if agentversion.Match(desired, ep.ReportedAgentVersion) {
		return nil
	}
	repo := s.cfg.GitHubRepo
	if repo == "" {
		repo = "DavidHoenisch/remotr"
	}
	return &agentUpgradePayload{
		Version:    desired,
		GitHubRepo: repo,
	}
}

func (s *Server) persistAgentUpgradeTelemetry(ctx context.Context, endpointID string, req syncRequest) {
	if s.cfg.Telemetry == nil {
		return
	}
	ver := req.AgentVersion
	phase := ""
	msg := ""
	if req.AgentUpgradeStatus != nil {
		phase = req.AgentUpgradeStatus.Phase
		msg = req.AgentUpgradeStatus.Message
	}
	clear := false
	if ver != "" && req.AgentUpgradeStatus != nil && req.AgentUpgradeStatus.Desired != "" {
		clear = agentversion.Match(req.AgentUpgradeStatus.Desired, ver) &&
			(req.AgentUpgradeStatus.Phase == "" || req.AgentUpgradeStatus.Phase == "completed")
	}
	if ver == "" && (req.AgentUpgradeStatus == nil || req.AgentUpgradeStatus.Phase == "") {
		return
	}
	if err := s.cfg.Telemetry.UpdateAgentUpgradeReport(ctx, endpointID, ver, phase, msg, clear); err != nil {
		slog.Warn("persist agent upgrade report", "endpoint", endpointID, "err", err)
	}
}
