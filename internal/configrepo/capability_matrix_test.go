package configrepo

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
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
