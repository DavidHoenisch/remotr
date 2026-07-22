package configrepo

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/providermatrix"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestValidateState_rejectsStaticallyImpossibleProviderTargets(t *testing.T) {
	tests := []struct {
		name    string
		config  models.Configuration
		wantErr string
	}{
		{
			name: "dnf is deferred",
			config: models.Configuration{Name: "base", TargetDistros: []types.Distro{types.Debian},
				Packages: []models.Package{{Name: "curl", Present: true, PM: types.Dnf}}},
			wantErr: "DNF provider is deferred to the RPM-family roadmap",
		},
		{
			name: "apt cannot target arch only",
			config: models.Configuration{Name: "base", TargetDistros: []types.Distro{types.Arch},
				Packages: []models.Package{{Name: "curl", Present: true, PM: types.Apt}}},
			wantErr: `provider "apt" is incompatible with target distro "Arch"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := models.State{SchemaVersion: 1, Configurations: []models.Configuration{tt.config}}
			err := ValidateState(state, "test")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "base/curl") {
				t.Fatalf("ValidateState() error = %v, want address and %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateStateRejectsDuplicateAndNonCanonicalTargets(t *testing.T) {
	tests := []struct {
		name    string
		config  models.Configuration
		wantErr string
	}{
		{name: "duplicate distro", config: models.Configuration{Name: "base", TargetDistros: []types.Distro{types.Ubuntu, types.Ubuntu}}, wantErr: `duplicate targetDistro "Ubuntu"`},
		{name: "duplicate architecture", config: models.Configuration{Name: "base", TargetArch: []types.Architecture{types.X86, types.X86}}, wantErr: `duplicate targetArch "x86"`},
		{name: "noncanonical distro", config: models.Configuration{Name: "base", TargetDistros: []types.Distro{"ubuntu"}}, wantErr: `invalid targetDistro`},
		{name: "noncanonical architecture", config: models.Configuration{Name: "base", TargetArch: []types.Architecture{"X86"}}, wantErr: `invalid targetArch`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateState(models.State{SchemaVersion: 1, Configurations: []models.Configuration{test.config}}, "test")
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ValidateState() error=%v, want %q", err, test.wantErr)
			}
		})
	}
}

// OS-AEC-117. Public seam: configuration validation.
func TestValidateStateAcceptsCanonicalPopOSTargetForPortableResource(t *testing.T) {
	state := models.State{SchemaVersion: 1, Configurations: []models.Configuration{{
		Name:          "pop-baseline",
		TargetDistros: []types.Distro{types.PopOS},
		Files:         []models.File{{Name: "banner", Path: "/etc/remotr-banner", Content: "managed"}},
	}}}
	if err := ValidateState(state, "test"); err != nil {
		t.Fatalf("ValidateState() rejected canonical PopOS target: %v", err)
	}
}

func TestValidateProviderReleaseRejectsMissingStalePartialAndMismatchedRows(t *testing.T) {
	state := models.State{SchemaVersion: 1, Configurations: []models.Configuration{{
		Name:          "base",
		TargetDistros: []types.Distro{types.Debian},
		TargetArch:    []types.Architecture{types.X86},
		Packages:      []models.Package{{Name: "remotr-fixture", Present: true, PM: types.Apt}},
		APTSigningKeys: []models.APTSigningKey{{
			Name: "vendor", Source: "https://keys.example.test/vendor.asc", Fingerprint: "0123456789ABCDEF0123456789ABCDEF01234567",
		}},
		APTRepositories: []models.APTRepository{{
			Name: "vendor", URL: "https://packages.example.test/debian", Suites: []string{"bookworm"}, Components: []string{"main"}, SigningKey: "vendor",
		}},
	}}}
	packageRow := providermatrix.Row{CapabilityID: "package", Provider: "package", Distribution: "debian", Release: "12", Architecture: "amd64", Backend: "apt", ContractRevision: "v1", Environment: "container", Status: "passing", Selectors: []string{"make:provider-matrix-apt-debian-12"}}
	repositoryRow := providermatrix.Row{CapabilityID: "repository", Provider: "repository", Distribution: "debian", Release: "12", Architecture: "amd64", Backend: "apt", ContractRevision: "v1", Environment: "container", Status: "passing", Selectors: []string{"make:provider-matrix-apt-repository-debian-12"}}

	tests := []struct {
		name    string
		rows    []providermatrix.Row
		wantErr string
	}{
		{"missing", nil, "missing passing provider evidence"},
		{"stale", func() []providermatrix.Row {
			row := packageRow
			row.Status = "untested"
			return []providermatrix.Row{row, repositoryRow}
		}(), "missing passing provider evidence"},
		{"partial", []providermatrix.Row{packageRow}, "repository/apt"},
		{"mismatched release", func() []providermatrix.Row {
			row := packageRow
			row.Release = "13"
			return []providermatrix.Row{row, repositoryRow}
		}(), "qualified provider identity"},
		{"mismatched architecture", func() []providermatrix.Row {
			row := packageRow
			row.Architecture = "arm64"
			return []providermatrix.Row{row, repositoryRow}
		}(), "qualified provider identity"},
		{"mismatched backend", func() []providermatrix.Row {
			row := packageRow
			row.Backend = "pacman"
			return []providermatrix.Row{row, repositoryRow}
		}(), "qualified provider identity"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateProviderRelease(state, providermatrix.Matrix{Version: 1, Rows: test.rows})
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ValidateProviderRelease() error = %v, want %q", err, test.wantErr)
			}
		})
	}
	if err := ValidateProviderRelease(state, providermatrix.Matrix{Version: 1, Rows: []providermatrix.Row{packageRow, repositoryRow}}); err != nil {
		t.Fatalf("qualified release rejected: %v", err)
	}
}

func TestValidateProviderReleaseAllowsUntargetedPortablePackage(t *testing.T) {
	state := models.State{SchemaVersion: 1, Configurations: []models.Configuration{{
		Name: "applications",
		Packages: []models.Package{{
			Name: "e2e/test-cli",
			PM:   types.Remotr,
		}},
	}}}

	if err := ValidateProviderRelease(state, providermatrix.Matrix{Version: 1}); err != nil {
		t.Fatalf("portable package release validation failed: %v", err)
	}
}

func TestValidateProviderReleaseReturnsStableDiagnosticCodes(t *testing.T) {
	base := models.Configuration{
		Name: "base", TargetDistros: []types.Distro{types.Debian}, TargetArch: []types.Architecture{types.X86},
		Packages: []models.Package{{Name: "curl", Present: true, PM: types.Apt}},
	}
	tests := []struct {
		name   string
		mutate func(*models.Configuration)
		matrix providermatrix.Matrix
		code   string
	}{
		{name: "missing distro", mutate: func(configuration *models.Configuration) { configuration.TargetDistros = nil }, matrix: providermatrix.Matrix{Version: 1}, code: ProviderReleaseTargetDistrosCode},
		{name: "missing architecture", mutate: func(configuration *models.Configuration) { configuration.TargetArch = nil }, matrix: providermatrix.Matrix{Version: 1}, code: ProviderReleaseTargetArchCode},
		{name: "unsupported evidence row", mutate: func(*models.Configuration) {}, matrix: providermatrix.Matrix{Version: 1}, code: ProviderReleaseEvidenceCode},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configuration := base
			test.mutate(&configuration)
			err := ValidateProviderRelease(models.State{SchemaVersion: 1, Configurations: []models.Configuration{configuration}}, test.matrix)
			if err == nil || ProviderReleaseErrorCode(err) != test.code || !strings.Contains(err.Error(), "base") {
				t.Fatalf("error=%v code=%q, want %q with safe configuration identity", err, ProviderReleaseErrorCode(err), test.code)
			}
		})
	}
}

func TestValidateState_requiresAPTRepositoryToDependOnItsSigningKey(t *testing.T) {
	state := models.State{SchemaVersion: 1, Configurations: []models.Configuration{{
		Name: "base",
		APTSigningKeys: []models.APTSigningKey{{
			Name: "vendor", Source: "https://keys.example.test/vendor.asc", Fingerprint: "0123456789ABCDEF0123456789ABCDEF01234567",
		}},
		APTRepositories: []models.APTRepository{{
			Name: "vendor-repository", URL: "https://packages.example.test/debian", Suites: []string{"stable"}, Components: []string{"main"}, SigningKey: "vendor",
		}},
	}}}
	if err := ValidateState(state, "test"); err == nil || !strings.Contains(err.Error(), "must explicitly depend on signing key \"base/vendor\"") {
		t.Fatalf("ValidateState() error = %v, want explicit signing-key dependency diagnostic", err)
	}
}
