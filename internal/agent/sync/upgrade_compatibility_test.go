package sync

import (
	"encoding/json"
	"testing"
)

// This exact response shape is decoded by the released v0.1.13 Response
// struct below and by the current client. v0.1.12 lacked AgentUpgrade and is
// therefore the documented out-of-band floor.
func TestCapabilityBlockedUpgradeResponseRetainsV0113DecoderCompatibility(t *testing.T) {
	raw := []byte(`{"releaseRef":"release-target","capabilityBlocked":{"targetReleaseRef":"release-target","missingRequirements":[{"id":"resource:package","revision":"package-v1"}]},"agentUpgrade":{"version":"v0.6.8","githubRepo":"DavidHoenisch/remotr"}}`)
	type v0113Response struct {
		ReleaseRef   string                   `json:"releaseRef,omitempty"`
		ArtifactYAML []byte                   `json:"artifactYaml,omitempty"`
		AgentUpgrade *AgentUpgradeInstruction `json:"agentUpgrade,omitempty"`
	}
	var legacy v0113Response
	if err := json.Unmarshal(raw, &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.AgentUpgrade == nil || legacy.AgentUpgrade.Version != "v0.6.8" || len(legacy.ArtifactYAML) != 0 {
		t.Fatalf("v0.1.13 decoder result = %+v", legacy)
	}
	var current Response
	if err := json.Unmarshal(raw, &current); err != nil {
		t.Fatal(err)
	}
	if current.AgentUpgrade == nil || current.CapabilityBlocked == nil || len(current.ArtifactYAML) != 0 {
		t.Fatalf("current decoder result = %+v", current)
	}
}
