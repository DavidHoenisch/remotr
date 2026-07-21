package ubuntuqualification_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
	"github.com/DavidHoenisch/remotr/internal/providermatrix"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/ubuntuqualification"
)

// Task 11.3: exception records are dispositions, not executable evidence. An
// exception selector cannot enter the provider matrix, and a not-applicable
// TDD record cannot be promoted from unadvertised to qualified.
func TestEvidenceExceptionCannotPromoteQualification(t *testing.T) {
	t.Run("exception is not a provider evidence selector", func(t *testing.T) {
		matrix := providermatrix.Matrix{
			Version:      1,
			Dependencies: providermatrix.AcceptedDependencyGates(),
			Rows: []providermatrix.Row{{
				CapabilityID: "host", Provider: "hosts-entry",
				Distribution: "ubuntu", Release: "24.04", Architecture: "amd64",
				Backend: "hosts-file", ContractRevision: "host-v1",
				Environment: "container", Status: "passing",
				Selectors: []string{"evidence-exception:EXC-002"},
			}},
		}
		if err := providermatrix.Validate(matrix); err == nil || !strings.Contains(err.Error(), "not an exact make or go-test target") {
			t.Fatalf("Validate(exception-only row) error = %v, want executable-selector rejection", err)
		}
		if providermatrix.Advertised(matrix, providermatrix.Claim{
			CapabilityID: "host", Provider: "hosts-entry",
			Distribution: "ubuntu", Release: "24.04", Architecture: "amd64",
			Backend: "hosts-file", ContractRevision: "host-v1", Environment: "container",
		}) {
			t.Fatal("exception-only row was advertised")
		}
		if _, err := capabilitydoc.NewDefaultGeneratorWithProviderMatrix([]int{1}, matrix); err == nil {
			t.Fatal("exception-only row reached capability publication")
		}
	})

	t.Run("not-applicable remains unadvertised", func(t *testing.T) {
		registry, err := resourceregistry.NewDefault()
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := ubuntuqualification.Load(
			filepath.Join("..", "..", "test", "qualification", "ubuntu-2404-applicators.yaml"),
			registry,
		)
		if err != nil {
			t.Fatal(err)
		}
		rowIndex := -1
		for index, row := range manifest.Rows {
			if row.Disposition == "qualified" {
				rowIndex = index
				break
			}
		}
		if rowIndex < 0 {
			t.Fatal("qualification manifest has no qualified row fixture")
		}

		candidate := manifest.Clone()
		candidate.Rows[rowIndex].TDD.Phase = "not-applicable"
		candidate.Rows[rowIndex].TDD.RedFailure = nil
		candidate.Rows[rowIndex].TDD.GreenResult = nil
		candidate.Rows[rowIndex].TDD.BroaderChecks = nil
		if err := ubuntuqualification.Validate(candidate, registry); err == nil || !strings.Contains(err.Error(), "qualified row requires verified TDD evidence") {
			t.Fatalf("Validate(not-applicable qualified row) error = %v, want verified-evidence gate", err)
		}
	})
}
