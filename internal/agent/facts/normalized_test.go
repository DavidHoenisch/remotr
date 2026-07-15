package facts_test

import (
	"reflect"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestFactsNormalizedFillsPortableProviderFacts(t *testing.T) {
	input := facts.Facts{
		Distro:        types.Ubuntu,
		DistroVersion: "24.04",
		Arch:          types.X86,
		Init:          facts.InitSystemd,
		Firewall:      facts.FirewallNftables,
		Network:       facts.NetworkManager,
		Security:      facts.SecurityAppArmor,
		Desktop:       []facts.DesktopBackend{facts.DesktopGSettings, facts.DesktopDconf, facts.DesktopDconf},
		Browser:       []facts.BrowserBackend{facts.BrowserFirefox, facts.BrowserChromium, facts.BrowserFirefox},
	}

	got := input.Normalized()
	if got.DistroFamily != facts.DistroFamilyDebian || got.Package != types.Apt {
		t.Fatalf("normalized family/package = %q/%q", got.DistroFamily, got.Package)
	}
	if got.DistroVersion != "24.04" || got.Init != facts.InitSystemd || got.Firewall != facts.FirewallNftables ||
		got.Network != facts.NetworkManager || got.Security != facts.SecurityAppArmor {
		t.Fatalf("normalized facts = %#v", got)
	}
	wantDesktop := []facts.DesktopBackend{facts.DesktopDconf, facts.DesktopGSettings}
	if !reflect.DeepEqual(got.Desktop, wantDesktop) {
		t.Fatalf("desktop = %v, want %v", got.Desktop, wantDesktop)
	}
	wantBrowser := []facts.BrowserBackend{facts.BrowserChromium, facts.BrowserFirefox}
	if !reflect.DeepEqual(got.Browser, wantBrowser) {
		t.Fatalf("browser = %v, want %v", got.Browser, wantBrowser)
	}
}
