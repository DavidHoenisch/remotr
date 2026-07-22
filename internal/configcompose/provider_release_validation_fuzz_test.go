package configcompose

import (
	"testing"

	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/providermatrix"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func FuzzProviderReleaseValidationClassificationIsShared(f *testing.F) {
	f.Add(uint8(0), uint8(0), uint8(0))
	f.Add(uint8(1), uint8(1), uint8(1))
	f.Fuzz(func(t *testing.T, distroChoice, architectureChoice, providerChoice uint8) {
		distros := [][]types.Distro{nil, {types.Debian}, {types.Ubuntu}, {types.Arch}}
		architectures := [][]types.Architecture{nil, {types.X86}, {types.Arm}}
		providers := []types.PackageManager{types.Apt, types.Pacman, types.Dnf}
		state := models.State{SchemaVersion: 1, Configurations: []models.Configuration{{
			Name:          "fuzz-target",
			TargetDistros: distros[int(distroChoice)%len(distros)],
			TargetArch:    architectures[int(architectureChoice)%len(architectures)],
			Packages:      []models.Package{{Name: "fixture", Present: true, PM: providers[int(providerChoice)%len(providers)]}},
		}}}
		matrix, err := providermatrix.Default()
		if err != nil {
			t.Fatal(err)
		}
		directErr := configrepo.ValidateProviderRelease(state, matrix)
		raw, err := resourceregistry.MarshalCanonical(state)
		if err != nil {
			t.Fatal(err)
		}
		sharedErr := ValidateRenderedProviderRelease(RenderedArtifact{TargetType: "fleet", TargetID: "fuzz", ArtifactType: "desired", YAML: raw}, matrix)
		if (directErr == nil) != (sharedErr == nil) || configrepo.ProviderReleaseErrorCode(directErr) != configrepo.ProviderReleaseErrorCode(sharedErr) {
			t.Fatalf("direct=%v (%s), shared=%v (%s)", directErr, configrepo.ProviderReleaseErrorCode(directErr), sharedErr, configrepo.ProviderReleaseErrorCode(sharedErr))
		}
	})
}
