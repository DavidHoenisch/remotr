package providermatrix

import (
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestDefaultMatrixIsTheEmbeddedRepositoryEvidence(t *testing.T) {
	want, err := Load(filepath.Join("..", "..", "test", "provider-matrix.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Default() differs from test/provider-matrix.yaml")
	}
}

func TestValidateAcceptsDistinctCompleteRows(t *testing.T) {
	matrix := Matrix{
		Version: 1,
		Rows: []Row{
			{
				CapabilityID:     "package",
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
				CapabilityID:     "package",
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

// OS-AEC-093: a provider-family name cannot substitute for the exact
// capability contract whose evidence would be advertised.
func TestValidateRequiresExactCapabilityIdentity(t *testing.T) {
	row := Row{
		Provider:         "filesystem",
		Distribution:     "ubuntu",
		Release:          "24.04",
		Architecture:     "amd64",
		Backend:          "posix",
		ContractRevision: "file-v1",
		Environment:      "container",
		Status:           "untested",
		Selectors:        []string{"make:provider-matrix-containers"},
	}
	if err := Validate(Matrix{Version: 1, Rows: []Row{row}}); err == nil || !strings.Contains(err.Error(), "capability_id is required") {
		t.Fatalf("Validate() error = %v, want missing exact capability identity", err)
	}
}

func TestValidateRequiresExecutableSelectors(t *testing.T) {
	row := Row{
		CapabilityID:     "file",
		Provider:         "filesystem",
		Distribution:     "ubuntu",
		Release:          "24.04",
		Architecture:     "amd64",
		Backend:          "posix",
		ContractRevision: "file-v1",
		Environment:      "container",
		Status:           "untested",
		Selectors:        []string{"shell:exit 0"},
	}
	if err := Validate(Matrix{Version: 1, Rows: []Row{row}}); err == nil || !strings.Contains(err.Error(), "not an exact make or go-test target") {
		t.Fatalf("Validate() error = %v, want non-executable selector rejection", err)
	}
}

func FuzzDecodeRequiresExactExecutableRows(f *testing.F) {
	f.Add([]byte("version: 1\nrows:\n  - {capability_id: file, provider: filesystem, distribution: ubuntu, release: '24.04', architecture: amd64, backend: posix, contract_revision: file-v1, environment: container, status: untested, selectors: [make:provider-matrix-containers]}\n"))
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 64<<10 {
			// test-exception: EXC-035
			t.Skip()
		}
		matrix, err := Decode(input)
		if err != nil {
			return
		}
		for _, row := range matrix.Rows {
			if row.CapabilityID == "" || row.Provider == "" || row.Backend == "" || row.ContractRevision == "" {
				t.Fatalf("Decode() accepted incomplete exact row: %#v", row)
			}
			for _, selector := range row.Selectors {
				if _, _, err := ResolveSelector(selector); err != nil {
					t.Fatalf("Decode() accepted non-executable selector %q: %v", selector, err)
				}
			}
		}
	})
}

func TestRepositoryMatrixTracksQualifiedPackageRowsAndLegacyDiscovery(t *testing.T) {
	matrix, err := Load(filepath.Join("..", "..", "test", "provider-matrix.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	wantDistributions := map[string]string{"debian": "12", "ubuntu": "24.04", "arch": "2026-07-06"}
	for distribution, release := range wantDistributions {
		for _, provider := range []string{"package", "repository"} {
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
	for distribution, release := range map[string]string{"debian": "12", "arch": "2026-07-06"} {
		for _, provider := range []string{"filesystem", "identity", "service"} {
			found := false
			for _, row := range matrix.Rows {
				if row.Provider == provider && row.Distribution == distribution && row.Release == release && row.Architecture == "amd64" && row.Environment == "container" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("missing legacy %s %s %s discovery row", distribution, release, provider)
			}
		}
	}
}

func TestCorePackageRowsUseExactReleaseSpecificExecutableSelectors(t *testing.T) {
	matrix, err := Load(filepath.Join("..", "..", "test", "provider-matrix.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	type rowIdentity struct {
		provider, distribution, release, backend string
	}
	want := map[rowIdentity][]string{
		{"package", "debian", "12", "apt"}:             {"make:provider-matrix-apt-debian-12"},
		{"repository", "debian", "12", "apt"}:          {"make:provider-matrix-apt-repository-debian-12"},
		{"package", "ubuntu", "24.04", "apt"}:          {"make:provider-matrix-apt-ubuntu-24-04"},
		{"repository", "ubuntu", "24.04", "apt"}:       {"make:provider-matrix-apt-repository-ubuntu-24-04"},
		{"package", "popos", "24.04", "apt"}:           {"make:provider-matrix-apt-popos-24-04"},
		{"package", "arch", "2026-07-06", "pacman"}:    {"make:provider-matrix-pacman-arch-2026-07-06"},
		{"package", "arch", "2026-07-06", "yay"}:       {"make:provider-matrix-aur-arch-2026-07-06"},
		{"repository", "arch", "2026-07-06", "pacman"}: {"make:provider-matrix-pacman-repository-arch-2026-07-06"},
	}
	for _, row := range matrix.Rows {
		identity := rowIdentity{row.Provider, row.Distribution, row.Release, row.Backend}
		selectors, required := want[identity]
		if !required {
			continue
		}
		if row.Status != "passing" || !slices.Equal(row.Selectors, selectors) {
			t.Errorf("core provider row %+v = status:%q selectors:%v, want passing %v", identity, row.Status, row.Selectors, selectors)
		}
		delete(want, identity)
	}
	for identity := range want {
		t.Errorf("missing core provider evidence row %+v", identity)
	}
}

func TestPassingCoreRowRequiresAndRunsItsExactEvidenceSet(t *testing.T) {
	row := Row{
		CapabilityID:     "package",
		Provider:         "package",
		Distribution:     "debian",
		Release:          "12",
		Architecture:     "amd64",
		Backend:          "apt",
		ContractRevision: "v1",
		Environment:      "container",
		Status:           "passing",
		Selectors:        []string{"go-test:./internal/providermatrix:^TestForgedPassingEvidence$"},
	}
	claim := Claim{
		CapabilityID:     row.CapabilityID,
		Provider:         row.Provider,
		Distribution:     row.Distribution,
		Release:          row.Release,
		Architecture:     row.Architecture,
		Backend:          row.Backend,
		ContractRevision: row.ContractRevision,
		Environment:      row.Environment,
	}
	matrix := Matrix{Version: 1, Rows: []Row{row}}
	var ran []string
	err := VerifyClaim(matrix, claim, func(selector string) error {
		ran = append(ran, selector)
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "complete evidence set") {
		t.Fatalf("forged passing row error = %v, want complete evidence set failure", err)
	}
	if len(ran) != 0 {
		t.Fatalf("forged selector ran before validation: %v", ran)
	}

	wantSelector := "make:provider-matrix-apt-debian-12"
	matrix.Rows[0].Selectors = []string{wantSelector}
	if err := VerifyClaim(matrix, claim, func(selector string) error {
		ran = append(ran, selector)
		return nil
	}); err != nil {
		t.Fatalf("verify exact evidence: %v", err)
	}
	if !slices.Equal(ran, []string{wantSelector}) {
		t.Fatalf("executed selectors = %v, want [%s]", ran, wantSelector)
	}

	evidenceFailure := errors.New("evidence failed")
	err = VerifyClaim(matrix, claim, func(string) error { return evidenceFailure })
	if !errors.Is(err, evidenceFailure) {
		t.Fatalf("failed evidence error = %v, want wrapped %v", err, evidenceFailure)
	}
}

func TestVerifyClaimSkipsNonpassingAndNonmatchingRowsWithoutRunningEvidence(t *testing.T) {
	claim := Claim{
		CapabilityID: "m-capability", Provider: "m-provider", Distribution: "m-distribution", Release: "m-release",
		Architecture: "m-architecture", Backend: "m-backend", ContractRevision: "m-revision", Environment: "container",
	}
	base := Row{
		CapabilityID: claim.CapabilityID, Provider: claim.Provider, Distribution: claim.Distribution, Release: claim.Release,
		Architecture: claim.Architecture, Backend: claim.Backend, ContractRevision: claim.ContractRevision, Environment: claim.Environment,
		Status: "passing", Selectors: []string{"go-test:./internal/providermatrix:^TestVerifyClaimSkipsNonpassingAndNonmatchingRowsWithoutRunningEvidence$"},
	}
	tests := map[string]func(*Row){
		"untested":                func(row *Row) { row.Status = "untested" },
		"failing":                 func(row *Row) { row.Status = "failing" },
		"nonmatching lower value": func(row *Row) { row.Distribution = "aaa" },
		"nonmatching upper value": func(row *Row) { row.Distribution = "zzz" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			row := base
			mutate(&row)
			runs := 0
			err := VerifyClaim(Matrix{Version: 1, Rows: []Row{row}}, claim, func(string) error {
				runs++
				return nil
			})
			if err == nil || runs != 0 {
				t.Fatalf("VerifyClaim() = error:%v runs:%d, want no matching evidence and zero runs", err, runs)
			}
		})
	}
}

func TestAdvertisedRequiresEveryClaimFieldToMatchExactly(t *testing.T) {
	base := Claim{
		CapabilityID: "m-capability", Provider: "m-provider", Distribution: "m-distribution", Release: "m-release",
		Architecture: "m-architecture", Backend: "m-backend", ContractRevision: "m-revision", Environment: "container",
	}
	matrix := Matrix{Version: 1, Rows: []Row{{
		CapabilityID: base.CapabilityID, Provider: base.Provider, Distribution: base.Distribution, Release: base.Release,
		Architecture: base.Architecture, Backend: base.Backend, ContractRevision: base.ContractRevision, Environment: base.Environment,
		Status: "passing", Selectors: []string{"go-test:./internal/providermatrix:^TestAdvertisedRequiresEveryClaimFieldToMatchExactly$"},
	}}}
	if !Advertised(matrix, base) {
		t.Fatal("exact claim was not advertised")
	}
	tests := map[string]func(*Claim, string){
		"capability ID":     func(claim *Claim, value string) { claim.CapabilityID = value },
		"provider":          func(claim *Claim, value string) { claim.Provider = value },
		"distribution":      func(claim *Claim, value string) { claim.Distribution = value },
		"release":           func(claim *Claim, value string) { claim.Release = value },
		"architecture":      func(claim *Claim, value string) { claim.Architecture = value },
		"backend":           func(claim *Claim, value string) { claim.Backend = value },
		"contract revision": func(claim *Claim, value string) { claim.ContractRevision = value },
		"environment":       func(claim *Claim, value string) { claim.Environment = value },
	}
	for field, mutate := range tests {
		for _, value := range []string{"aaa", "zzz"} {
			t.Run(field+" "+value, func(t *testing.T) {
				claim := base
				mutate(&claim, value)
				if Advertised(matrix, claim) {
					t.Fatalf("claim with %s=%q was advertised", field, value)
				}
			})
		}
	}
}

func TestPassingCoreEvidenceRejectsEverySelectorShapeBoundary(t *testing.T) {
	const want = "make:provider-matrix-apt-debian-12"
	base := Row{
		CapabilityID: "package", Provider: "package", Distribution: "debian", Release: "12", Architecture: "amd64", Backend: "apt",
		ContractRevision: "v1", Environment: "container", Status: "passing", Selectors: []string{want},
	}
	claim := Claim{CapabilityID: base.CapabilityID, Provider: base.Provider, Distribution: base.Distribution, Release: base.Release, Architecture: base.Architecture, Backend: base.Backend, ContractRevision: base.ContractRevision, Environment: base.Environment}
	tests := map[string][]string{
		"missing":     nil,
		"extra":       {want, "go-test:./internal/providermatrix:^TestPassingCoreEvidenceRejectsEverySelectorShapeBoundary$"},
		"lower value": {"aaa"},
		"upper value": {"zzz"},
	}
	for name, selectors := range tests {
		t.Run(name, func(t *testing.T) {
			row := base
			row.Selectors = selectors
			matrix := Matrix{Version: 1, Rows: []Row{row}}
			if err := Validate(matrix); err == nil {
				t.Fatal("Validate() accepted malformed passing selector evidence")
			}
			if Advertised(matrix, claim) {
				t.Fatal("Advertised() accepted malformed passing selector evidence")
			}
		})
	}
}

func TestNonCoreProviderNamesRemainOutsideCoreNativeEvidenceGate(t *testing.T) {
	for _, provider := range []string{"aaa", "zzz"} {
		t.Run(provider, func(t *testing.T) {
			row := Row{
				CapabilityID: "future-capability", Provider: provider, Distribution: "future", Release: "1", Architecture: "amd64", Backend: "apt",
				ContractRevision: "v1", Environment: "container", Status: "passing",
				Selectors: []string{"go-test:./internal/providermatrix:^TestNonCoreProviderNamesRemainOutsideCoreNativeEvidenceGate$"},
			}
			if err := Validate(Matrix{Version: 1, Rows: []Row{row}}); err != nil {
				t.Fatalf("Validate() rejected non-core provider: %v", err)
			}
		})
	}
}

func TestPassingCoreRowRequiresAnExactlyQualifiedIdentity(t *testing.T) {
	base := Row{
		CapabilityID:     "package",
		Provider:         "package",
		Distribution:     "debian",
		Release:          "12",
		Architecture:     "amd64",
		Backend:          "apt",
		ContractRevision: "v1",
		Environment:      "container",
		Status:           "passing",
		Selectors:        []string{"make:provider-matrix-apt-debian-12"},
	}
	tests := map[string]func(*Row){
		"release":            func(row *Row) { row.Release = "13" },
		"architecture":       func(row *Row) { row.Architecture = "arm64" },
		"backend":            func(row *Row) { row.Backend = "dnf" },
		"non-native backend": func(row *Row) { row.Backend = "flatpak" },
		"contract revision":  func(row *Row) { row.ContractRevision = "v2" },
		"environment":        func(row *Row) { row.Environment = "vm" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			row := base
			mutate(&row)
			err := Validate(Matrix{Version: 1, Rows: []Row{row}})
			if err == nil || !strings.Contains(err.Error(), "qualified provider identity") {
				t.Fatalf("Validate() error = %v, want qualified provider identity failure", err)
			}
		})
	}
}

func TestResolveSelectorUsesOnlyExactMakeOrGoTestTargets(t *testing.T) {
	tests := []struct {
		selector string
		name     string
		args     []string
	}{
		{"make:provider-matrix-apt-debian-12", "make", []string{"provider-matrix-apt-debian-12"}},
		{"go-test:./internal/providermatrix:^TestVerifyClaim$", "go", []string{"test", "-mod=vendor", "./internal/providermatrix", "-run", "^TestVerifyClaim$", "-count=1"}},
	}
	for _, test := range tests {
		t.Run(test.selector, func(t *testing.T) {
			name, args, err := ResolveSelector(test.selector)
			if err != nil {
				t.Fatal(err)
			}
			if name != test.name || !slices.Equal(args, test.args) {
				t.Fatalf("ResolveSelector() = %q %v, want %q %v", name, args, test.name, test.args)
			}
		})
	}
	for _, selector := range []string{"provider-matrix-apt-debian-12", "make:", "make:all extra", "go-test:./internal/providermatrix", "shell:exit 0"} {
		t.Run("reject "+selector, func(t *testing.T) {
			if _, _, err := ResolveSelector(selector); err == nil {
				t.Fatalf("ResolveSelector(%q) succeeded", selector)
			}
		})
	}
}

func TestDeferredPackageFamiliesRemainUnadvertised(t *testing.T) {
	for _, backend := range []string{"dnf", "dnf4", "dnf5", "rpm", "rpm-ostree", "apk", "zypper", "snap"} {
		t.Run(backend, func(t *testing.T) {
			row := Row{
				CapabilityID:     "package",
				Provider:         "package",
				Distribution:     "future-distribution",
				Release:          "1",
				Architecture:     "amd64",
				Backend:          backend,
				ContractRevision: "v1",
				Environment:      "container",
				Status:           "passing",
				Selectors:        []string{"make:provider-matrix-containers"},
			}
			matrix := Matrix{Version: 1, Rows: []Row{row}}
			if err := Validate(matrix); err == nil {
				t.Fatalf("passing %s row was accepted without a qualified evidence set", backend)
			}
			claim := Claim{CapabilityID: row.CapabilityID, Provider: row.Provider, Distribution: row.Distribution, Release: row.Release, Architecture: row.Architecture, Backend: row.Backend, ContractRevision: row.ContractRevision, Environment: row.Environment}
			if Advertised(matrix, claim) {
				t.Fatalf("deferred %s row was advertised", backend)
			}
		})
	}
}

func TestValidateRejectsIncompleteOrDuplicateRows(t *testing.T) {
	row := Row{
		CapabilityID:     "firewall",
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
		CapabilityID:     "package",
		Provider:         "apt-package",
		Distribution:     "debian",
		Release:          "12",
		Architecture:     "amd64",
		Backend:          "apt",
		ContractRevision: "v1",
		Environment:      "container",
	}
	matrix := Matrix{Version: 1, Rows: []Row{{
		CapabilityID:     claim.CapabilityID,
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
	for _, status := range []string{"untested", "failing"} {
		matrix.Rows[0].Status = status
		if Advertised(matrix, claim) {
			t.Fatalf("%s matrix row was advertised", status)
		}
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
