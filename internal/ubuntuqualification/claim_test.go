package ubuntuqualification_test

import (
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
	"github.com/DavidHoenisch/remotr/internal/providermatrix"
	"github.com/DavidHoenisch/remotr/internal/types"
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

func TestPassingRowWaitsForSharedDependencies(t *testing.T) {
	// OS-AEC-102 focused red observed: a passing exact file row was emitted as
	// resource:file even though the matrix recorded no accepted dependencies.
	row := providermatrix.Row{
		CapabilityID: "file", Provider: "file", Distribution: "ubuntu", Release: "24.04",
		Architecture: "amd64", Backend: "posix", ContractRevision: "file-v1", Environment: "container",
		Status: "passing", Selectors: []string{"go-test:./internal/ubuntuqualification:^TestPassingRowWaitsForSharedDependencies$"},
	}
	matrix := providermatrix.Matrix{Version: 1, Rows: []providermatrix.Row{row}}
	claim := providermatrix.Claim{
		CapabilityID: row.CapabilityID, Provider: row.Provider,
		Distribution: row.Distribution, Release: row.Release, Architecture: row.Architecture,
		Backend: row.Backend, ContractRevision: row.ContractRevision, Environment: row.Environment,
	}
	if !providermatrix.Advertised(matrix, claim) {
		t.Fatal("passing provider evidence was not recorded")
	}

	generator, err := capabilitydoc.NewDefaultGeneratorWithProviderMatrix([]int{1}, matrix)
	if err != nil {
		t.Fatal(err)
	}
	document, err := generator.Generate(facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "24.04", Arch: types.X86,
	}, "v1")
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range document.Capabilities {
		if capability.ID == "resource:file" {
			t.Fatalf("passing row was published without accepted shared dependencies: %+v", document.Capabilities)
		}
	}

	for _, test := range []struct {
		name   string
		mutate func(*providermatrix.DependencyGates)
	}{
		{"execution contract", func(gates *providermatrix.DependencyGates) { gates.ExecutionContract = false }},
		{"capability delivery", func(gates *providermatrix.DependencyGates) { gates.CapabilityDelivery = false }},
		{"testing foundation", func(gates *providermatrix.DependencyGates) { gates.TestingFoundation = false }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := matrix
			candidate.Dependencies = providermatrix.AcceptedDependencyGates()
			test.mutate(&candidate.Dependencies)
			if providermatrix.AdvertisedForPublication(candidate, claim) {
				t.Fatal("passing row was published while its shared dependency was incomplete")
			}
			if !providermatrix.Advertised(candidate, claim) {
				t.Fatal("dependency gate discarded recorded passing evidence")
			}
		})
	}

	packageRow := providermatrix.Row{
		CapabilityID: "package", Provider: "package", Distribution: "ubuntu", Release: "24.04",
		Architecture: "amd64", Backend: "apt", ContractRevision: "v1", Environment: "container",
		Status: "passing", Selectors: []string{"make:provider-matrix-apt-ubuntu-24-04"},
	}
	packageMatrix := providermatrix.Matrix{
		Version:      1,
		Dependencies: providermatrix.AcceptedDependencyGates(),
		Rows:         []providermatrix.Row{packageRow},
	}
	packageMatrix.Dependencies.PackageProviders = false
	packageClaim := providermatrix.Claim{
		CapabilityID: packageRow.CapabilityID, Provider: packageRow.Provider,
		Distribution: packageRow.Distribution, Release: packageRow.Release, Architecture: packageRow.Architecture,
		Backend: packageRow.Backend, ContractRevision: packageRow.ContractRevision, Environment: packageRow.Environment,
	}
	if !providermatrix.Advertised(packageMatrix, packageClaim) {
		t.Fatal("passing package evidence was not recorded")
	}
	if providermatrix.AdvertisedForPublication(packageMatrix, packageClaim) {
		t.Fatal("passing package row was published while the package-provider dependency was incomplete")
	}
}
