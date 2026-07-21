package ubuntuqualification_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/ubuntuqualification"
)

func TestTDDRecordGatesProductionCorrections(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ubuntuqualification.Load(filepath.Join("..", "..", "test", "qualification", "ubuntu-2404-applicators.yaml"), registry)
	if err != nil {
		t.Fatalf("load repository qualification manifest: %v", err)
	}
	if len(manifest.Rows) == 0 {
		t.Fatal("qualification manifest has no rows for TDD state-machine fixtures")
	}

	approvedSeams := map[string]bool{
		"configuration-cli": true, "operator-cli-admin-api": true, "authenticated-sync": true,
		"composed-agent-execution": true, "provider-contract": true,
		"system-safety-recovery": true, "observable-performance": true,
	}
	for _, row := range manifest.Rows {
		record := row.TDD
		if !slices.Contains(row.GoverningIDs, record.GoverningID) || !approvedSeams[record.PublicSeam] ||
			strings.TrimSpace(record.ExpectedResult) == "" || len(record.EvidenceLayers) == 0 ||
			record.FinalDisposition != row.Disposition {
			t.Errorf("%s/%s has incomplete TDD record: %+v", row.CapabilityID, row.Backend, record)
		}
	}
	fixture := manifest.Clone()
	fixture.Rows[0].Disposition = "blocked"
	fixture.Rows[0].Reason = "synthetic blocked row for TDD state-machine validation"
	fixture.Rows[0].TDD.FinalDisposition = "blocked"
	blockedRow := 0

	tests := []struct {
		name   string
		mutate func(*ubuntuqualification.Row)
		want   string
	}{
		{
			name: "red phase lacks observed failure",
			mutate: func(row *ubuntuqualification.Row) {
				row.TDD.Phase = "red-observed"
				row.TDD.RedFailure = nil
			},
			want: "red-observed TDD phase requires an observed red failure",
		},
		{
			name: "green phase lacks result",
			mutate: func(row *ubuntuqualification.Row) {
				row.TDD.Phase = "green"
				red := "focused provider test failed before production changes"
				row.TDD.RedFailure = &red
				row.TDD.GreenResult = nil
			},
			want: "green TDD phase requires red and green results",
		},
		{
			name: "verified phase lacks broader checks",
			mutate: func(row *ubuntuqualification.Row) {
				row.TDD.Phase = "verified"
				red, green := "focused provider test failed before production changes", "focused provider test passed after the minimum correction"
				row.TDD.RedFailure, row.TDD.GreenResult = &red, &green
				row.TDD.BroaderChecks = nil
			},
			want: "verified TDD phase requires broader checks",
		},
		{
			name: "final disposition differs",
			mutate: func(row *ubuntuqualification.Row) {
				row.TDD.FinalDisposition = "qualified"
			},
			want: "TDD final disposition",
		},
		{
			name: "qualification before verified TDD",
			mutate: func(row *ubuntuqualification.Row) {
				row.Disposition = "qualified"
				row.TDD.FinalDisposition = "qualified"
				row.TDD.Phase = "planned"
				row.TDD.RedFailure = nil
				row.TDD.GreenResult = nil
				row.TDD.BroaderChecks = nil
			},
			want: "qualified row requires verified TDD evidence",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := fixture.Clone()
			test.mutate(&candidate.Rows[blockedRow])
			err := ubuntuqualification.Validate(candidate, registry)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}
}

// OS-AEC-100: the real Ubuntu auditd fixture contradicted the earlier focused
// assumption that augenrules --check validates staged syscall names. The exact
// row stays blocked through red-observed and focused-green phases and can be
// promoted only after the public-provider correction and broader VM evidence.
func TestRealFixtureContradictionBlocksRow(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ubuntuqualification.Load(filepath.Join("..", "..", "test", "qualification", "ubuntu-2404-applicators.yaml"), registry)
	if err != nil {
		t.Fatal(err)
	}

	rowIndex := -1
	for index, row := range manifest.Rows {
		if row.CapabilityID == "auditRules" && row.Backend == "auditd" {
			rowIndex = index
			break
		}
	}
	if rowIndex < 0 {
		t.Fatal("missing exact auditRules/auditd qualification row")
	}
	original := manifest.Rows[rowIndex]
	if original.TDD.RedFailure == nil ||
		!strings.Contains(*original.TDD.RedFailure, "first real Ubuntu run") ||
		!strings.Contains(*original.TDD.RedFailure, "invalid staged syscall replaced the persistent fragment") ||
		!strings.Contains(*original.TDD.RedFailure, "focused public-provider regression") ||
		!slices.Contains(original.Selectors, "make:provider-matrix-vm-system-safety") {
		t.Fatalf("auditRules/auditd does not retain the real-fixture contradiction: %+v", original.TDD)
	}

	contradicted := manifest.Clone()
	row := &contradicted.Rows[rowIndex]
	row.Disposition = "blocked"
	row.Reason = "real Ubuntu auditd fixture contradicted focused validation evidence"
	row.TDD.Phase = "red-observed"
	row.TDD.GreenResult = nil
	row.TDD.BroaderChecks = nil
	row.TDD.FinalDisposition = "blocked"
	if err := ubuntuqualification.Validate(contradicted, registry); err != nil {
		t.Fatalf("red-observed exact row should remain validly blocked: %v", err)
	}
	for index := range manifest.Rows {
		if index != rowIndex && contradicted.Rows[index].Disposition != manifest.Rows[index].Disposition {
			t.Fatalf("contradiction changed sibling row %s/%s", manifest.Rows[index].CapabilityID, manifest.Rows[index].Backend)
		}
	}

	promotedBeforeGreen := contradicted.Clone()
	promotedBeforeGreen.Rows[rowIndex].Disposition = "qualified"
	promotedBeforeGreen.Rows[rowIndex].TDD.FinalDisposition = "qualified"
	if err := ubuntuqualification.Validate(promotedBeforeGreen, registry); err == nil || !strings.Contains(err.Error(), "qualified row requires verified TDD evidence") {
		t.Fatalf("red-observed row promotion error = %v, want verified TDD gate", err)
	}

	focusedGreen := contradicted.Clone()
	green := *original.TDD.GreenResult
	focusedGreen.Rows[rowIndex].TDD.Phase = "green"
	focusedGreen.Rows[rowIndex].TDD.GreenResult = &green
	if err := ubuntuqualification.Validate(focusedGreen, registry); err != nil {
		t.Fatalf("focused-green exact row should remain validly blocked: %v", err)
	}
	promotedBeforeBroader := focusedGreen.Clone()
	promotedBeforeBroader.Rows[rowIndex].Disposition = "qualified"
	promotedBeforeBroader.Rows[rowIndex].TDD.FinalDisposition = "qualified"
	if err := ubuntuqualification.Validate(promotedBeforeBroader, registry); err == nil || !strings.Contains(err.Error(), "qualified row requires verified TDD evidence") {
		t.Fatalf("focused-green row promotion error = %v, want broader-evidence gate", err)
	}

	verifiedWithoutBroader := promotedBeforeBroader.Clone()
	verifiedWithoutBroader.Rows[rowIndex].TDD.Phase = "verified"
	if err := ubuntuqualification.Validate(verifiedWithoutBroader, registry); err == nil || !strings.Contains(err.Error(), "verified TDD phase requires broader checks") {
		t.Fatalf("verified row without broader evidence error = %v, want TDD broader-check gate", err)
	}

	verifiedWithWrongBroader := verifiedWithoutBroader.Clone()
	verifiedWithWrongBroader.Rows[rowIndex].TDD.BroaderChecks = []string{"go-test:./internal/applicators/auditrules"}
	if err := ubuntuqualification.Validate(verifiedWithWrongBroader, registry); err == nil || !strings.Contains(err.Error(), "task 9.10 Ubuntu VM evidence") {
		t.Fatalf("verified row without broader VM evidence error = %v, want exact VM evidence gate", err)
	}

	corrected := contradicted.Clone()
	corrected.Rows[rowIndex] = original
	if err := ubuntuqualification.Validate(corrected, registry); err != nil {
		t.Fatalf("corrected row with focused and broader VM evidence was rejected: %v", err)
	}
}
