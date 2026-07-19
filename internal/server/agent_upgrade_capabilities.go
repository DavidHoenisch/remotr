package server

import (
	"strconv"
	"strings"

	agentsync "github.com/DavidHoenisch/remotr/internal/agent/sync"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

type agentUpgradeCapabilityProfile struct {
	CapabilityDocument bool
	Schemas            map[int]bool
	Contracts          map[string]string
}

// Upgrade profiles describe only agent-shipped protocol and applicator
// contracts. Runtime providers are intentionally never granted by version.
var agentUpgradeCapabilityProfiles = map[string]agentUpgradeCapabilityProfile{
	"v1.2.4": {
		CapabilityDocument: true,
		Schemas:            map[int]bool{1: true},
		Contracts:          map[string]string{"resource:package": "package-v1"},
	},
}

func (s *Server) compatibleBlockedUpgradeInstruction(endpoint registry.Endpoint, missing []agentsync.MissingRequirement) *agentUpgradePayload {
	instruction := s.agentUpgradeInstruction(endpoint)
	if instruction == nil {
		return nil
	}
	profile, ok := agentUpgradeCapabilityProfiles[instruction.Version]
	if !ok {
		return nil
	}
	for _, requirement := range missing {
		switch {
		case requirement.ID == "capability-document":
			if !profile.CapabilityDocument {
				return nil
			}
		case strings.HasPrefix(requirement.ID, "schema:"):
			version, err := strconv.Atoi(strings.TrimPrefix(requirement.ID, "schema:"))
			if err != nil || !profile.Schemas[version] {
				return nil
			}
		case strings.HasPrefix(requirement.ID, "resource:"):
			if profile.Contracts[requirement.ID] != requirement.Revision {
				return nil
			}
		default:
			// Provider requirements are endpoint runtime evidence and cannot be
			// satisfied by an agent-version mapping.
			return nil
		}
	}
	return instruction
}
