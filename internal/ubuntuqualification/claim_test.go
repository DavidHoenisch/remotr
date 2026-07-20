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

// OS-AEC-094: a broad family row is discovery evidence only. It cannot stand
// in for either of the independently versioned resource contracts below.
func TestQualificationClaimRejectsBroadFamilyEvidence(t *testing.T) {
	base := providermatrix.Row{
		Provider: "filesystem", Distribution: "ubuntu", Release: "24.04", Architecture: "amd64",
		Backend: "posix", Environment: "container", Status: "passing",
		Selectors: []string{"go-test:./internal/ubuntuqualification:^TestQualificationClaimRejectsBroadFamilyEvidence$"},
	}
	broad := base
	broad.CapabilityID = "filesystem"
	broad.ContractRevision = "v1"
	file := base
	file.CapabilityID = "file"
	file.ContractRevision = "file-v1"
	matrix := providermatrix.Matrix{Version: 1, Rows: []providermatrix.Row{broad, file}}

	claim := func(capabilityID, revision string) providermatrix.Claim {
		return providermatrix.Claim{
			CapabilityID: capabilityID, Provider: base.Provider,
			Distribution: base.Distribution, Release: base.Release, Architecture: base.Architecture,
			Backend: base.Backend, ContractRevision: revision, Environment: base.Environment,
		}
	}
	if !providermatrix.Advertised(matrix, claim("file", "file-v1")) {
		t.Fatal("exact passing file row was not advertised")
	}
	if providermatrix.Advertised(matrix, claim("download", "download-v1")) {
		t.Fatal("broad filesystem row advertised sibling download contract")
	}
}
