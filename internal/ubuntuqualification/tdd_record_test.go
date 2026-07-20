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

	tests := []struct {
		name   string
		mutate func(*ubuntuqualification.Row)
		want   string
	}{
		{
			name: "red phase lacks observed failure",
			mutate: func(row *ubuntuqualification.Row) {
				row.TDD.Phase = "red-observed"
			},
			want: "red-observed TDD phase requires an observed red failure",
		},
		{
			name: "green phase lacks result",
			mutate: func(row *ubuntuqualification.Row) {
				row.TDD.Phase = "green"
				red := "focused provider test failed before production changes"
				row.TDD.RedFailure = &red
			},
			want: "green TDD phase requires red and green results",
		},
		{
			name: "verified phase lacks broader checks",
			mutate: func(row *ubuntuqualification.Row) {
				row.TDD.Phase = "verified"
				red, green := "focused provider test failed before production changes", "focused provider test passed after the minimum correction"
				row.TDD.RedFailure, row.TDD.GreenResult = &red, &green
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
			},
			want: "qualified row requires verified TDD evidence",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := manifest.Clone()
			test.mutate(&candidate.Rows[0])
			err := ubuntuqualification.Validate(candidate, registry)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}
}
