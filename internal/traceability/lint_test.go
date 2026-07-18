package traceability

import "testing"

func TestValidateReportsTraceabilityIntegrityFailures(t *testing.T) {
	registry := PrefixRegistry{Version: 1, Prefixes: map[string]PrefixOwnership{"OS-AEC": {Change: "change", Capability: "capability"}}}
	inventory := []Scenario{
		{Change: "change", Capability: "capability", VerificationID: "OS-AEC-001", Path: "one.md", ScenarioLine: 1},
		{Change: "change", Capability: "capability", VerificationID: "OS-AEC-001", Path: "two.md", ScenarioLine: 2},
		{Change: "change", Capability: "capability", VerificationID: "OS-AEC-02", Path: "bad.md", ScenarioLine: 3},
		{Change: "change", Capability: "capability", Path: "missing.md", ScenarioLine: 4},
	}
	manifest := Manifest{Version: 1, Environments: map[string]string{"in-process": "test"}, Scenarios: map[string]ManifestEntry{
		"OS-AEC-001": {Source: PrefixOwnership{Change: "wrong", Capability: "capability"}, Lifecycle: "verified", VerificationClasses: []string{"unit"}, Selectors: []string{"invalid"}, Environments: []string{"missing"}},
		"OS-AEC-999": {Source: PrefixOwnership{Change: "change", Capability: "capability"}, Lifecycle: "planned", VerificationClasses: []string{"unit"}, DispositionReason: "orphan"},
	}}
	issues := Validate(inventory, registry, manifest)
	if len(issues) < 7 {
		for _, issue := range issues {
			t.Log(issue)
		}
		t.Fatalf("issue count = %d, want at least 7", len(issues))
	}
}

func TestValidateAcceptsPlannedManifestEntry(t *testing.T) {
	registry := PrefixRegistry{Version: 1, Prefixes: map[string]PrefixOwnership{"OS-AEC": {Change: "change", Capability: "capability"}}}
	inventory := []Scenario{{Change: "change", Capability: "capability", VerificationID: "OS-AEC-001", Path: "spec.md", ScenarioLine: 1}}
	manifest := Manifest{Version: 1, Environments: map[string]string{"in-process": "test"}, Scenarios: map[string]ManifestEntry{
		"OS-AEC-001": {Source: PrefixOwnership{Change: "change", Capability: "capability"}, Lifecycle: "planned", VerificationClasses: []string{"unit"}, DispositionReason: "not implemented"},
	}}
	if issues := Validate(inventory, registry, manifest); len(issues) != 0 {
		t.Fatalf("issues = %v", issues)
	}
}

func TestValidateAllowsOneSelectorForMultipleIDsAndMultipleLayersForOneID(t *testing.T) {
	registry := PrefixRegistry{Version: 1, Prefixes: map[string]PrefixOwnership{"OS-AEC": {Change: "change", Capability: "capability"}}}
	inventory := []Scenario{
		{Change: "change", Capability: "capability", VerificationID: "OS-AEC-001", Path: "one.md", ScenarioLine: 1},
		{Change: "change", Capability: "capability", VerificationID: "OS-AEC-002", Path: "two.md", ScenarioLine: 1},
	}
	sharedSelector := "go-test:./internal/traceability:^TestShared$"
	manifest := Manifest{Version: 1, Environments: map[string]string{"in-process": "unit", "container": "integration"}, Scenarios: map[string]ManifestEntry{
		"OS-AEC-001": {Source: PrefixOwnership{Change: "change", Capability: "capability"}, Lifecycle: "verified", VerificationClasses: []string{"unit", "contract"}, Selectors: []string{sharedSelector, "make:provider-contract"}, Environments: []string{"in-process", "container"}},
		"OS-AEC-002": {Source: PrefixOwnership{Change: "change", Capability: "capability"}, Lifecycle: "verified", VerificationClasses: []string{"unit"}, Selectors: []string{sharedSelector}, Environments: []string{"in-process"}},
	}}
	if issues := Validate(inventory, registry, manifest); len(issues) != 0 {
		t.Fatalf("issues = %v", issues)
	}
}

func TestValidateAcceptsAuthorizedCapabilityModifierLineage(t *testing.T) {
	registry := PrefixRegistry{Version: 1, Prefixes: map[string]PrefixOwnership{
		"OS-AEC": {Change: "umbrella", Capability: "capability", Modifiers: []string{"child"}},
	}}
	inventory := []Scenario{
		{Change: "umbrella", Capability: "capability", Operation: "added", VerificationID: "OS-AEC-001", Path: "umbrella.md", ScenarioLine: 1},
		{Change: "child", Capability: "capability", Operation: "modified", VerificationID: "OS-AEC-001", Path: "child.md", ScenarioLine: 1},
		{Change: "child", Capability: "capability", Operation: "added", VerificationID: "OS-AEC-080", Path: "child.md", ScenarioLine: 2},
	}
	manifest := Manifest{Version: 1, Environments: map[string]string{"in-process": "test"}, Scenarios: map[string]ManifestEntry{
		"OS-AEC-001": {Source: PrefixOwnership{Change: "umbrella", Capability: "capability"}, Lifecycle: "planned", VerificationClasses: []string{"unit"}, DispositionReason: "refinement pending"},
		"OS-AEC-080": {Source: PrefixOwnership{Change: "umbrella", Capability: "capability"}, Lifecycle: "planned", VerificationClasses: []string{"unit"}, DispositionReason: "implementation pending"},
	}}
	if issues := Validate(inventory, registry, manifest); len(issues) != 0 {
		t.Fatalf("issues = %v", issues)
	}
}

func TestValidateRejectsUnauthorizedAndCompetingCapabilityModifiers(t *testing.T) {
	registry := PrefixRegistry{Version: 1, Prefixes: map[string]PrefixOwnership{
		"OS-AEC": {Change: "umbrella", Capability: "capability", Modifiers: []string{"child", "other-child"}},
	}}
	inventory := []Scenario{
		{Change: "unauthorized", Capability: "capability", Operation: "added", VerificationID: "OS-AEC-080", Path: "unauthorized.md", ScenarioLine: 1},
		{Change: "child", Capability: "capability", Operation: "modified", VerificationID: "OS-AEC-001", Path: "child.md", ScenarioLine: 1},
		{Change: "other-child", Capability: "capability", Operation: "modified", VerificationID: "OS-AEC-001", Path: "other.md", ScenarioLine: 1},
	}
	manifest := Manifest{Version: 1, Environments: map[string]string{"in-process": "test"}, Scenarios: map[string]ManifestEntry{
		"OS-AEC-001": {Source: PrefixOwnership{Change: "umbrella", Capability: "capability"}, Lifecycle: "planned", VerificationClasses: []string{"unit"}, DispositionReason: "refinement pending"},
		"OS-AEC-080": {Source: PrefixOwnership{Change: "umbrella", Capability: "capability"}, Lifecycle: "planned", VerificationClasses: []string{"unit"}, DispositionReason: "implementation pending"},
	}}
	issues := Validate(inventory, registry, manifest)
	if len(issues) < 2 {
		t.Fatalf("issues = %v, want unauthorized and competing modifier failures", issues)
	}
}
