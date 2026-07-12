package capabilitymatrix

import (
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func FuzzValidateStaticPackageTarget(f *testing.F) {
	f.Add("apt", "Arch")
	f.Add("dnf", "Debian")
	f.Add("pacman", "Arch")
	f.Fuzz(func(t *testing.T, provider, distro string) {
		if len(provider) > 32 || len(distro) > 32 {
			return
		}
		configuration := models.Configuration{
			Name:          "base",
			TargetDistros: []types.Distro{types.Distro(distro)},
		}
		resource := &models.Package{Name: "package", PM: types.PackageManager(provider)}
		err := ValidateStatic(1, configuration, resource)
		if provider == string(types.Dnf) && err == nil {
			t.Fatal("DNF must remain statically unavailable")
		}
		if provider == string(types.Apt) && distro == string(types.Arch) && err == nil {
			t.Fatal("APT cannot target Arch")
		}
		if provider == string(types.Pacman) && distro == string(types.Arch) && err != nil {
			t.Fatalf("Pacman should target Arch: %v", err)
		}
	})
}
