package providerregistry_test

import (
	"reflect"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/providerregistry"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestDefaultRegistryResolvesNormalizedBackends(t *testing.T) {
	registry, err := providerregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	endpoint := facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "24.04", Arch: types.X86,
		Init: facts.InitSystemd, Firewall: facts.FirewallNftables,
		Network: facts.NetworkManager, Security: facts.SecurityAppArmor,
		Desktop: []facts.DesktopBackend{facts.DesktopDconf, facts.DesktopGSettings},
	}.Normalized()

	got := registry.Resolve(endpoint)
	want := map[providerregistry.Capability][]string{
		providerregistry.CapabilityPackage:  {"apt"},
		providerregistry.CapabilityInit:     {"systemd"},
		providerregistry.CapabilityFirewall: {"nftables"},
		providerregistry.CapabilityNetwork:  {"network-manager"},
		providerregistry.CapabilitySecurity: {"apparmor"},
		providerregistry.CapabilityDesktop:  {"dconf", "gsettings"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolve() = %#v, want %#v", got, want)
	}
	for _, providers := range got {
		for _, provider := range providers {
			if provider == "dnf" {
				t.Fatal("deferred DNF provider must not be registered")
			}
		}
	}
}
