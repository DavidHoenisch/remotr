package providermatrix

import "testing"

func TestValidateAcceptsDistinctCompleteRows(t *testing.T) {
	matrix := Matrix{
		Version: 1,
		Rows: []Row{
			{
				Provider:         "apt-package",
				Distribution:     "debian",
				Release:          "12",
				Architecture:     "amd64",
				Backend:          "apt",
				ContractRevision: "v1",
				Environment:      "container",
				Status:           "passing",
				Selectors:        []string{"go-test:./internal/applicators/packages/apt:^TestApplicator_ConformsForPresenceAndRemoval$"},
			},
			{
				Provider:         "apt-package",
				Distribution:     "debian",
				Release:          "12",
				Architecture:     "arm64",
				Backend:          "apt",
				ContractRevision: "v1",
				Environment:      "container",
				Status:           "untested",
				Selectors:        []string{"make:provider-matrix"},
			},
		},
	}
	if err := Validate(matrix); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsIncompleteOrDuplicateRows(t *testing.T) {
	row := Row{
		Provider:         "firewall-rule",
		Distribution:     "debian",
		Release:          "12",
		Architecture:     "amd64",
		Backend:          "nftables",
		ContractRevision: "v1",
		Environment:      "container",
		Status:           "passing",
		Selectors:        []string{"go-test:./internal/applicators/firewall:^TestApplicator_ConformsForAuditRule$"},
	}
	if err := Validate(Matrix{Version: 1, Rows: []Row{row, row}}); err == nil {
		t.Fatal("duplicate row was accepted")
	}
	row.Selectors = nil
	if err := Validate(Matrix{Version: 1, Rows: []Row{row}}); err == nil {
		t.Fatal("row without selectors was accepted")
	}
	row.Selectors = []string{"go-test:./internal/applicators/firewall:^TestApplicator_ConformsForAuditRule$"}
	row.Environment = "host"
	if err := Validate(Matrix{Version: 1, Rows: []Row{row}}); err == nil {
		t.Fatal("unknown environment was accepted")
	}
	row.Environment = "container"
	row.Status = "unknown"
	if err := Validate(Matrix{Version: 1, Rows: []Row{row}}); err == nil {
		t.Fatal("unknown status was accepted")
	}
}

func TestAdvertisedRequiresMatchingPassingEvidence(t *testing.T) {
	claim := Claim{
		Provider:         "apt-package",
		Distribution:     "debian",
		Release:          "12",
		Architecture:     "amd64",
		Backend:          "apt",
		ContractRevision: "v1",
		Environment:      "container",
	}
	matrix := Matrix{Version: 1, Rows: []Row{{
		Provider:         claim.Provider,
		Distribution:     claim.Distribution,
		Release:          claim.Release,
		Architecture:     claim.Architecture,
		Backend:          claim.Backend,
		ContractRevision: claim.ContractRevision,
		Environment:      claim.Environment,
		Status:           "untested",
		Selectors:        []string{"go-test:./internal/applicators/packages/apt:^TestApplicator_ConformsForPresenceAndRemoval$"},
	}}}
	if Advertised(matrix, claim) {
		t.Fatal("untested matrix row was advertised")
	}

	matrix.Rows[0].Status = "passing"
	if !Advertised(matrix, claim) {
		t.Fatal("matching passing matrix row was not advertised")
	}

	claim.Release = "13"
	if Advertised(matrix, claim) {
		t.Fatal("non-matching matrix row was advertised")
	}
}
