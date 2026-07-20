package configrepo

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func FuzzValidatePackagePolicyByProvider(f *testing.F) {
	f.Add("unknown", "present", "", "x86", uint16(0), "")
	f.Add("apt", "present", "2.0.0-1", "x86", uint16(0b0000001111), "")
	f.Add("pacman", "purged", "", "x86", uint16(0), "")
	f.Add("apt", "absent", "2.0.0-1", "ARM", uint16(0b0011111111), "")
	f.Add("flatpak", "present", "", "x86", uint16(0b0000000100), "company")

	f.Fuzz(func(t *testing.T, provider, lifecycle, version, architecture string, flags uint16, providerOption string) {
		if len(provider) > 32 || len(lifecycle) > 32 || len(version) > 128 || len(architecture) > 32 || len(providerOption) > 256 {
			return
		}
		pkg := models.Package{
			ResourceMeta:       models.ResourceMeta{Lifecycle: models.Lifecycle(lifecycle)},
			Name:               "fixture",
			Present:            lifecycle == string(models.LifecyclePresent),
			Version:            version,
			PM:                 types.PackageManager(provider),
			Arch:               types.Architecture(architecture),
			AllowUpgrade:       fuzzBoolPointer(flags, 0, 1),
			AllowDowngrade:     fuzzBoolPointer(flags, 2, 3),
			Hold:               fuzzBoolPointer(flags, 4, 5),
			RefreshCache:       flags&(1<<6) != 0,
			RemoveDependencies: flags&(1<<7) != 0,
			NonInteractive:     fuzzBoolPointer(flags, 8, 9),
		}
		if pkg.PM == types.Flatpak {
			pkg.FlatpakRemote = providerOption
		}
		if pkg.PM == types.Pwa {
			pkg.PWAURL = providerOption
		}
		if pkg.PM == types.Yay {
			pkg.AURBuildUser = providerOption
		}
		pkg.NormalizeLifecycle()

		state := models.State{Configurations: []models.Configuration{{Name: "base", Packages: []models.Package{pkg}}}}
		first := ValidateState(state, "fuzz.yaml")
		second := ValidateState(state, "fuzz.yaml")
		if (first == nil) != (second == nil) || first != nil && first.Error() != second.Error() {
			t.Fatalf("package validation is nondeterministic: first=%v second=%v", first, second)
		}
		if first != nil {
			if len(first.Error()) > 1024 {
				t.Fatalf("package validation diagnostic is unbounded: %d bytes", len(first.Error()))
			}
			return
		}

		if !acceptedPackageProvider(pkg.PM) {
			t.Fatalf("unknown provider %q was accepted", pkg.PM)
		}
		if pkg.Lifecycle != models.LifecyclePresent && pkg.Lifecycle != models.LifecycleAbsent && pkg.Lifecycle != models.LifecyclePurged {
			t.Fatalf("unknown lifecycle %q was accepted", pkg.Lifecycle)
		}
		if pkg.Lifecycle == models.LifecyclePurged && pkg.PM != types.Apt {
			t.Fatalf("purge was accepted for provider %q", pkg.PM)
		}
		if pkg.NonInteractive != nil && !*pkg.NonInteractive {
			t.Fatal("interactive native transaction was accepted")
		}
		native := pkg.PM == types.Apt || pkg.PM == types.Pacman
		if !native && (pkg.AllowUpgrade != nil || pkg.AllowDowngrade != nil || pkg.Hold != nil || pkg.RefreshCache || pkg.RemoveDependencies) {
			t.Fatalf("native policy was accepted for provider %q", pkg.PM)
		}
		if pkg.Hold != nil && pkg.PM != types.Apt {
			t.Fatalf("hold was accepted for provider %q", pkg.PM)
		}
		if pkg.Lifecycle != models.LifecyclePresent && (strings.TrimSpace(pkg.Version) != "" || pkg.AllowUpgrade != nil || pkg.AllowDowngrade != nil || pkg.Hold != nil || pkg.RefreshCache) {
			t.Fatalf("present-only intent was accepted for lifecycle %q", pkg.Lifecycle)
		}
		if pkg.Lifecycle == models.LifecyclePresent && pkg.RemoveDependencies {
			t.Fatal("removal policy was accepted for present lifecycle")
		}
		if strings.TrimSpace(pkg.Version) == "" && (pkg.AllowUpgrade != nil || pkg.AllowDowngrade != nil) {
			t.Fatal("version transition policy was accepted without an exact version")
		}
	})
}

func fuzzBoolPointer(flags uint16, presentBit, valueBit uint) *bool {
	if flags&(1<<presentBit) == 0 {
		return nil
	}
	value := flags&(1<<valueBit) != 0
	return &value
}

func acceptedPackageProvider(provider types.PackageManager) bool {
	switch provider {
	case "", types.Apt, types.Pacman, types.Flatpak, types.Pwa, types.Remotr:
		return true
	default:
		return false
	}
}
