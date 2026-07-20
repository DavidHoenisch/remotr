package ubuntuqualification_test

import (
	"testing"

	"github.com/DavidHoenisch/remotr/internal/providermatrix"
)

// OS-AEC-093: only a complete, exact passing tuple may authorize a support
// claim. Presence of implementation code or a non-passing/malformed row is
// deliberately insufficient.
//
// Focused red observed: malformed passing rows with missing selectors or the
// non-executable "shell:exit 0" selector were advertised as supported.
func TestQualificationClaimRequiresExactPassingTuple(t *testing.T) {
	base := providermatrix.Row{
		CapabilityID:     "file",
		Provider:         "filesystem",
		Distribution:     "ubuntu",
		Release:          "24.04",
		Architecture:     "amd64",
		Backend:          "posix",
		ContractRevision: "file-v1",
		Environment:      "container",
		Status:           "passing",
		Selectors:        []string{"go-test:./internal/ubuntuqualification:^TestQualificationClaimRequiresExactPassingTuple$"},
	}
	claim := providermatrix.Claim{
		CapabilityID:     base.CapabilityID,
		Provider:         base.Provider,
		Distribution:     base.Distribution,
		Release:          base.Release,
		Architecture:     base.Architecture,
		Backend:          base.Backend,
		ContractRevision: base.ContractRevision,
		Environment:      base.Environment,
	}
	tests := []struct {
		name   string
		rows   []providermatrix.Row
		mutate func(*providermatrix.Row)
		want   bool
	}{
		{name: "missing"},
		{name: "untested", mutate: func(row *providermatrix.Row) { row.Status = "untested" }},
		{name: "planned", mutate: func(row *providermatrix.Row) { row.Status = "planned" }},
		{name: "skipped", mutate: func(row *providermatrix.Row) { row.Status = "skipped" }},
		{name: "failing", mutate: func(row *providermatrix.Row) { row.Status = "failing" }},
		{name: "stale revision", mutate: func(row *providermatrix.Row) { row.ContractRevision = "file-v0" }},
		{name: "missing selectors", mutate: func(row *providermatrix.Row) { row.Selectors = nil }},
		{name: "non-executable selector", mutate: func(row *providermatrix.Row) { row.Selectors = []string{"shell:exit 0"} }},
		{name: "exact passing", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows := test.rows
			if test.name != "missing" {
				row := base
				if test.mutate != nil {
					test.mutate(&row)
				}
				rows = []providermatrix.Row{row}
			}
			if got := providermatrix.Advertised(providermatrix.Matrix{Version: 1, Rows: rows}, claim); got != test.want {
				t.Fatalf("Advertised() = %t, want %t for rows %#v", got, test.want, rows)
			}
		})
	}
}
