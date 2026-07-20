package configrepo

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestValidateStateRequiresPacmanRepositorySigningKeyDependencies(t *testing.T) {
	base := models.Configuration{
		Name: "base", TargetDistros: []types.Distro{types.Arch},
		PacmanSigningKeys: []models.PacmanSigningKey{{
			Name: "vendor", Source: "https://keys.example.test/vendor.asc",
			Fingerprint: "0123456789ABCDEF0123456789ABCDEF01234567",
		}},
		PacmanRepositories: []models.PacmanRepository{{
			Name: "vendor-repository", Servers: []string{"https://mirror.example.test/$repo/os/$arch"}, Architecture: "x86_64",
			SignatureLevel: models.PacmanSignatureRequired, SigningKeys: []string{"vendor"},
		}},
	}

	missingDependency := models.State{SchemaVersion: 1, Configurations: []models.Configuration{base}}
	if err := ValidateState(missingDependency, "test"); err == nil || !strings.Contains(err.Error(), `must explicitly depend on signing key "base/vendor"`) {
		t.Fatalf("ValidateState() = %v, want Pacman signing-key dependency error", err)
	}

	base.PacmanRepositories[0].DependsOn = []string{"base/vendor"}
	valid := models.State{SchemaVersion: 1, Configurations: []models.Configuration{base}}
	if err := ValidateState(valid, "test"); err != nil {
		t.Fatalf("ValidateState(valid Pacman trust graph) = %v", err)
	}
}
