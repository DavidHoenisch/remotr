package providermatrix

import (
	"path/filepath"
	"testing"
)

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

func TestRepositoryMatrixTracksCoreProviderFamiliesOnSupportedDistributions(t *testing.T) {
	matrix, err := Load(filepath.Join("..", "..", "test", "provider-matrix.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	wantDistributions := map[string]string{"debian": "12", "ubuntu": "24.04", "arch": "2026-07-06"}
	wantProviders := []string{"package", "filesystem", "identity", "service", "repository"}
	for distribution, release := range wantDistributions {
		for _, provider := range wantProviders {
			found := false
			for _, row := range matrix.Rows {
				if row.Provider == provider && row.Distribution == distribution && row.Release == release && row.Architecture == "amd64" && row.Environment == "container" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("missing %s %s %s container row", distribution, release, provider)
			}
		}
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
