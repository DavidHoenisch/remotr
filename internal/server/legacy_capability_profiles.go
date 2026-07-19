package server

import "github.com/DavidHoenisch/remotr/internal/capabilitydoc"

const legacyCapabilityProfileMappingVersion = 1

type legacyCapabilityProfile struct {
	ArtifactSchemaVersions []int
	Capabilities           []capabilitydoc.Capability
}

// knownLegacyCapabilityProfiles is deliberately exact and reviewed. Agent
// versions absent from this table do not inherit a known version's contracts.
var knownLegacyCapabilityProfiles = map[string]legacyCapabilityProfile{
	"v0.1.12": {
		ArtifactSchemaVersions: []int{0},
		Capabilities: []capabilitydoc.Capability{
			{ID: "resource:command", Revision: "command-v1"},
		},
	},
}

func knownLegacyCapabilityDocument(agentVersion string) (capabilitydoc.Document, bool) {
	profile, ok := knownLegacyCapabilityProfiles[agentVersion]
	if !ok {
		return capabilitydoc.Document{}, false
	}
	document, err := (capabilitydoc.Document{
		DocumentVersion:        capabilitydoc.CurrentDocumentVersion,
		ArtifactSchemaVersions: append([]int(nil), profile.ArtifactSchemaVersions...),
		Capabilities:           append([]capabilitydoc.Capability(nil), profile.Capabilities...),
		AgentVersion:           agentVersion,
	}).WithCanonicalDigest()
	if err != nil {
		return capabilitydoc.Document{}, false
	}
	return document, true
}
