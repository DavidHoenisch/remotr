package capabilitydoc

import (
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestGeneratorDerivesRegisteredContractsAndCurrentFacts(t *testing.T) {
	generator, err := NewDefaultGenerator([]int{0, 1})
	if err != nil {
		t.Fatal(err)
	}
	document, err := generator.Generate(facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "24.04", Arch: types.X86,
		Init: facts.InitSystemd, Package: types.Apt,
		Firewall: facts.FirewallNftables, Network: facts.NetworkManager,
		Security: facts.SecurityAppArmor,
		Desktop:  []facts.DesktopBackend{facts.DesktopGSettings, facts.DesktopDconf},
		Browser:  []facts.BrowserBackend{facts.BrowserFirefox},
	}, "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}

	for _, expected := range []Capability{
		{ID: "resource:package", Revision: "package-v1"},
		{ID: "provider:package/apt", Revision: "1"},
		{ID: "provider:init/systemd", Revision: "1"},
		{ID: "provider:desktop/dconf", Revision: "1"},
	} {
		if !slices.Contains(document.Capabilities, expected) {
			t.Fatalf("capabilities do not contain %+v: %+v", expected, document.Capabilities)
		}
	}
	for _, unavailable := range []Capability{
		{ID: "provider:package/pacman", Revision: "1"},
		{ID: "provider:package/dnf", Revision: "1"},
	} {
		if slices.Contains(document.Capabilities, unavailable) {
			t.Fatalf("capabilities unexpectedly contain %+v", unavailable)
		}
	}
	for _, expected := range []Fact{
		{Key: "distro", Value: "ubuntu"},
		{Key: "distro-version", Value: "24.04"},
		{Key: "architecture", Value: "x86"},
		{Key: "init", Value: "systemd"},
		{Key: "desktop", Value: "dconf"},
	} {
		if !slices.Contains(document.Facts, expected) {
			t.Fatalf("facts do not contain %+v: %+v", expected, document.Facts)
		}
	}
	if document.Digest == "" || document.AgentVersion != "v1.2.3" || !slices.Equal(document.ArtifactSchemaVersions, []int{0, 1}) {
		t.Fatalf("generated document = %+v", document)
	}
	if err := document.Validate(); err != nil {
		t.Fatalf("generated document is invalid: %v", err)
	}
}
