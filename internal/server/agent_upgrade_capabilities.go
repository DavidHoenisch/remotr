package server

import (
	"slices"

	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/releasecatalog"
)

func (s *Server) compatibleBlockedUpgradeInstruction(endpoint registry.Endpoint, document *capabilitydoc.Document) *agentUpgradePayload {
	instruction := s.agentUpgradeInstruction(endpoint)
	if instruction == nil {
		return nil
	}
	release, approved, err := releasecatalog.AgentReleaseByVersion(instruction.Version)
	if err != nil || !approved || !blockedUpgradeEligible(release, document) {
		return nil
	}
	return instruction
}

func blockedUpgradeEligible(release releasecatalog.AgentRelease, document *capabilitydoc.Document) bool {
	if !release.UpgradeEligible || release.Revoked || release.Integrity != "sha256-manifest" {
		return false
	}
	// Remotr agents are Linux binaries. Architecture is current endpoint
	// evidence; release metadata never stands in for runtime provider support.
	if !slices.Contains(release.Platforms, "linux") || document == nil {
		return false
	}
	architecture := ""
	for _, fact := range document.Facts {
		if fact.Key == "architecture" {
			architecture = fact.Value
			break
		}
	}
	if architecture == "" || !slices.Contains(release.Architectures, architecture) {
		return false
	}
	return true
}
