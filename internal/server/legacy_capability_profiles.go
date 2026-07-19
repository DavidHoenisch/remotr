package server

import "github.com/DavidHoenisch/remotr/internal/capabilitydoc"

const legacyCapabilityProfileMappingVersion = 1

type legacyCapabilityProfile struct {
	ArtifactSchemaVersions []int
	Capabilities           []capabilitydoc.Capability
}

var minimalLegacyCapabilityProfile = legacyCapabilityProfile{
	ArtifactSchemaVersions: []int{0},
	Capabilities: []capabilitydoc.Capability{
		{ID: "resource:command", Revision: "command-v1"},
	},
}

// knownLegacyCapabilityProfiles is deliberately exact and reviewed. Agent
// versions absent from this table do not inherit a known version's contracts.
var knownLegacyCapabilityProfiles = map[string]legacyCapabilityProfile{
	"v0.1.12": minimalLegacyCapabilityProfile,
}

// Known modern versions are classified only as requiring current capability
// evidence. This mapping never grants them schemas, providers, or resources.
var knownModernCapabilityDocumentVersions = map[string]bool{
	"dev": true, "v1.2.3": true,
}

func knownLegacyCapabilityDocument(agentVersion string) (capabilitydoc.Document, bool) {
	profile, ok := knownLegacyCapabilityProfiles[agentVersion]
	if !ok {
		return capabilitydoc.Document{}, false
	}
	document, err := capabilityDocumentForLegacyProfile(profile, agentVersion)
	if err != nil {
		return capabilitydoc.Document{}, false
	}
	return document, true
}

func minimalLegacyCapabilityDocument() capabilitydoc.Document {
	document, _ := capabilityDocumentForLegacyProfile(minimalLegacyCapabilityProfile, "legacy-unknown")
	return document
}

func capabilityDocumentForLegacyProfile(profile legacyCapabilityProfile, agentVersion string) (capabilitydoc.Document, error) {
	return (capabilitydoc.Document{
		DocumentVersion:        capabilitydoc.CurrentDocumentVersion,
		ArtifactSchemaVersions: append([]int(nil), profile.ArtifactSchemaVersions...),
		Capabilities:           append([]capabilitydoc.Capability(nil), profile.Capabilities...),
		AgentVersion:           agentVersion,
	}).WithCanonicalDigest()
}

func isKnownModernCapabilityDocumentVersion(agentVersion string) bool {
	return knownModernCapabilityDocumentVersions[agentVersion]
}
