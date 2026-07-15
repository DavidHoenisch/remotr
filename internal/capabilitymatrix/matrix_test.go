package capabilitymatrix

import (
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestNetworkProfileRequiresAndMatchesItsSelectedProvider(t *testing.T) {
	profile := &models.NetworkProfileResource{Provider: models.NetworkProviderNetplan}
	requirements := Requirements(models.ResourceKindNetworkProfile, profile)
	if !slices.Contains(requirements, "provider:network/netplan") || slices.Contains(requirements, "provider:network/network-manager") {
		t.Fatalf("profile requirements = %v", requirements)
	}
	if err := CheckRuntime(profile, facts.Facts{Network: facts.NetworkNetplan}); err != nil {
		t.Fatal(err)
	}
	if err := CheckRuntime(profile, facts.Facts{Network: facts.NetworkSystemdNetwork}); err == nil {
		t.Fatal("mismatched runtime backend was accepted")
	}
}

func TestAppArmorProfilesAreLimitedToUbuntuAppArmorEndpoints(t *testing.T) {
	resource := &models.AppArmorProfileResource{Name: "service", Profile: "service", Content: "profile service {}\n", Mode: models.AppArmorEnforce}
	if requirements := Requirements(models.ResourceKindAppArmorProfile, resource); !slices.Contains(requirements, "provider:security/apparmor") {
		t.Fatalf("AppArmor requirements = %v", requirements)
	}
	if err := ValidateStatic(1, models.Configuration{TargetDistros: []types.Distro{types.Ubuntu}}, resource); err != nil {
		t.Fatal(err)
	}
	if err := ValidateStatic(1, models.Configuration{TargetDistros: []types.Distro{types.Debian}}, resource); err == nil {
		t.Fatal("Debian target advertised unsupported AppArmor provider")
	}
	if err := CheckRuntime(resource, facts.Facts{Distro: types.Ubuntu, Security: facts.SecurityAppArmor}); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []facts.Facts{
		{Distro: types.Ubuntu, Security: facts.SecuritySELinux},
		{Distro: types.Debian, Security: facts.SecurityAppArmor},
	} {
		if err := CheckRuntime(resource, endpoint); err == nil {
			t.Fatalf("unsupported endpoint accepted: %+v", endpoint)
		}
	}
}

func TestBrowserPolicyRequiresSelectedInstalledBrowser(t *testing.T) {
	resource := &models.BrowserPolicyResource{Browser: models.BrowserChromium}
	if requirements := Requirements(models.ResourceKindBrowserPolicy, resource); !slices.Contains(requirements, "provider:browser/chromium") {
		t.Fatalf("browser requirements = %v", requirements)
	}
	if err := CheckRuntime(resource, facts.Facts{Browser: []facts.BrowserBackend{facts.BrowserChromium}}); err != nil {
		t.Fatal(err)
	}
	if err := CheckRuntime(resource, facts.Facts{Browser: []facts.BrowserBackend{facts.BrowserFirefox}}); err == nil {
		t.Fatal("mismatched browser capability was accepted")
	}
}

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
