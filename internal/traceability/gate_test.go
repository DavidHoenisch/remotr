package traceability

import (
	"errors"
	"testing"
)

func TestAdvertisementIssuesRejectsPlannedAndFailingEvidence(t *testing.T) {
	manifest := Manifest{Scenarios: map[string]ManifestEntry{
		"OS-AEC-001": {Source: PrefixOwnership{Change: "change", Capability: "capability"}, Lifecycle: "planned"},
	}}
	if issues := AdvertisementIssues(manifest, "change", "capability", func(string) error { return nil }); len(issues) != 1 {
		t.Fatalf("planned issues = %v", issues)
	}

	manifest.Scenarios["OS-AEC-001"] = ManifestEntry{Source: PrefixOwnership{Change: "change", Capability: "capability"}, Lifecycle: "verified", Selectors: []string{"go-test:./internal/traceability:^TestGate$"}, Environments: []string{"in-process"}}
	if issues := AdvertisementIssues(manifest, "change", "capability", func(string) error { return errors.New("test failed") }); len(issues) != 1 {
		t.Fatalf("failing evidence issues = %v", issues)
	}
}

func TestAdvertisementIssuesAcceptsPassingVerifiedEvidence(t *testing.T) {
	manifest := Manifest{Scenarios: map[string]ManifestEntry{
		"OS-AEC-001": {Source: PrefixOwnership{Change: "change", Capability: "capability"}, Lifecycle: "verified", Selectors: []string{"go-test:./internal/traceability:^TestGate$"}, Environments: []string{"in-process"}},
	}}
	if issues := AdvertisementIssues(manifest, "change", "capability", func(string) error { return nil }); len(issues) != 0 {
		t.Fatalf("issues = %v", issues)
	}
}
