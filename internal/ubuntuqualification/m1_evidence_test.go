package ubuntuqualification_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/ubuntuqualification"
)

// OS-AEC-097: the M1 container result is evidence for only the two exact
// ordinary contracts exercised by that selector. It cannot stand in for an
// access, service, or VM safety/recovery result.
func TestM1OrdinaryContainerEvidenceIsExactAndRiskBounded(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ubuntuqualification.Load(filepath.Join("..", "..", "test", "qualification", "ubuntu-2404-applicators.yaml"), registry)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]struct {
		backend  string
		revision string
	}{
		"file":     {backend: "posix", revision: "file-v1"},
		"download": {backend: "https", revision: "download-v1"},
	}
	found := make(map[string]bool, len(want))
	for _, row := range manifest.Rows {
		expectation, isM1Ordinary := want[row.CapabilityID]
		if isM1Ordinary && row.Backend == expectation.backend {
			found[row.CapabilityID] = true
			if row.ContractRevision != expectation.revision || row.Environment != "container" || row.Risk != "ordinary" {
				t.Errorf("%s evidence tuple is not exact ordinary container evidence: %+v", row.CapabilityID, row)
			}
			if row.TDD.GoverningID != "OS-AEC-097" || row.TDD.PublicSeam != "provider-contract" || row.TDD.Phase != "verified" {
				t.Errorf("%s provider-contract evidence is incomplete: %+v", row.CapabilityID, row.TDD)
			}
			if !slices.Contains(row.Selectors, "make:provider-matrix-containers") || !hasContainerResult(row.TDD.BroaderChecks) {
				t.Errorf("%s lacks the pinned Ubuntu container result: %+v", row.CapabilityID, row.TDD.BroaderChecks)
			}
			continue
		}

		if strings.HasPrefix(row.Environment, "vm-") && hasContainerResult(row.TDD.BroaderChecks) {
			t.Errorf("container-only M1 evidence was attached to VM-qualified %s/%s", row.CapabilityID, row.Backend)
		}
	}
	for capabilityID := range want {
		if !found[capabilityID] {
			t.Errorf("missing exact verified M1 evidence for %s", capabilityID)
		}
	}
}

func hasContainerResult(checks []string) bool {
	for _, check := range checks {
		if check == "make:provider-matrix-containers on pinned Ubuntu 24.04 amd64" {
			return true
		}
	}
	return false
}
