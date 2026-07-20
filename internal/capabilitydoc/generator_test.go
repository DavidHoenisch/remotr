package capabilitydoc

import (
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/providermatrix"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestGeneratorDerivesRegisteredContractsAndCurrentFacts(t *testing.T) {
	matrix := providermatrix.Matrix{Version: 1, Dependencies: providermatrix.AcceptedDependencyGates(), Rows: []providermatrix.Row{
		{CapabilityID: "package", Provider: "package", Distribution: "ubuntu", Release: "24.04", Architecture: "amd64", Backend: "apt", ContractRevision: "v1", Environment: "container", Status: "passing", Selectors: []string{"make:provider-matrix-apt-ubuntu-24-04"}},
		{CapabilityID: "repository", Provider: "repository", Distribution: "ubuntu", Release: "24.04", Architecture: "amd64", Backend: "apt", ContractRevision: "v1", Environment: "container", Status: "passing", Selectors: []string{"make:provider-matrix-apt-repository-ubuntu-24-04"}},
	}}
	generator, err := NewDefaultGeneratorWithProviderMatrix([]int{0, 1}, matrix)
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
	} {
		capability, found := capabilityWithID(document.Capabilities, expected.ID)
		if !found || capability.Revision != expected.Revision {
			t.Fatalf("capabilities do not contain %+v: %+v", expected, document.Capabilities)
		}
	}
	for _, expected := range []struct {
		id       string
		features []string
	}{
		{"provider:package/apt", []string{"lifecycle:present", "lifecycle:absent", "lifecycle:purged", "version:exact", "policy:hold", "policy:downgrade"}},
		{"provider:repository/apt", []string{"repository:present", "repository:absent", "repository:disabled", "repository:scoped-credentials"}},
		{"provider:trust/apt", []string{"trust:full-fingerprint", "trust:scoped-keyring"}},
	} {
		capability, found := capabilityWithID(document.Capabilities, expected.id)
		if !found {
			t.Fatalf("capabilities do not contain %q: %+v", expected.id, document.Capabilities)
		}
		for _, feature := range expected.features {
			if !slices.Contains(capability.Features, feature) {
				t.Fatalf("capability %q features = %v, missing %q", expected.id, capability.Features, feature)
			}
		}
	}
	for _, unavailable := range []Capability{
		{ID: "provider:package/pacman", Revision: "1"},
		{ID: "provider:package/dnf", Revision: "1"},
		{ID: "provider:init/systemd", Revision: "1"},
		{ID: "provider:desktop/dconf", Revision: "1"},
	} {
		if capability, found := capabilityWithID(document.Capabilities, unavailable.ID); found && capability.Revision == unavailable.Revision {
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

func TestGeneratorPublishesOnlyQualifiedExactRows(t *testing.T) {
	// OS-AEC-094 focused red observed: the generator published every registered
	// resource, including resource:download, from this file-only evidence set.
	// Task 3.6 focused red observed: fact-only provider entries were published
	// without rows, while the exact sysctl row omitted provider:kernel/sysctl.
	matrix := providermatrix.Matrix{Version: 1, Dependencies: providermatrix.AcceptedDependencyGates(), Rows: []providermatrix.Row{
		{
			CapabilityID: "file", Provider: "filesystem", Distribution: "ubuntu", Release: "24.04",
			Architecture: "amd64", Backend: "posix", ContractRevision: "file-v1", Environment: "container",
			Status: "passing", Selectors: []string{"go-test:./internal/capabilitydoc:^TestGeneratorPublishesOnlyQualifiedExactRows$"},
		},
		{
			CapabilityID: "filesystem", Provider: "filesystem", Distribution: "ubuntu", Release: "24.04",
			Architecture: "amd64", Backend: "posix", ContractRevision: "v1", Environment: "container",
			Status: "passing", Selectors: []string{"go-test:./internal/ubuntuqualification:^TestQualificationClaimRejectsBroadFamilyEvidence$"},
		},
		{
			CapabilityID: "service", Provider: "service", Distribution: "ubuntu", Release: "24.04",
			Architecture: "amd64", Backend: "systemd", ContractRevision: "service-state-v1", Environment: "vm",
			Status: "passing", Selectors: []string{"go-test:./internal/capabilitydoc:^TestGeneratorPublishesOnlyQualifiedExactRows$"},
		},
		{
			CapabilityID: "sysctl", Provider: "sysctl", Distribution: "ubuntu", Release: "24.04",
			Architecture: "amd64", Backend: "procfs", ContractRevision: "sysctl-v1", Environment: "vm",
			Status: "passing", Selectors: []string{"go-test:./internal/capabilitydoc:^TestGeneratorPublishesOnlyQualifiedExactRows$"},
		},
	}}
	generator, err := NewDefaultGeneratorWithProviderMatrix([]int{1}, matrix)
	if err != nil {
		t.Fatal(err)
	}
	document, err := generator.Generate(facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "24.04", Arch: types.X86,
		Init: facts.InitSystemd, Firewall: facts.FirewallNftables, Network: facts.NetworkManager,
		Security: facts.SecurityAppArmor, Desktop: []facts.DesktopBackend{facts.DesktopDconf},
		Browser: []facts.BrowserBackend{facts.BrowserFirefox},
	}, "v1")
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"provider:init/systemd": "1", "provider:kernel/sysctl": "1",
		"resource:file": "file-v1", "resource:service": "service-state-v1", "resource:sysctl": "sysctl-v1",
	}
	for _, capability := range document.Capabilities {
		revision, ok := want[capability.ID]
		if !ok {
			t.Errorf("capability %q has no exact passing source row: %+v", capability.ID, document.Capabilities)
			continue
		}
		if capability.Revision != revision {
			t.Errorf("capability %q revision = %q, want %q", capability.ID, capability.Revision, revision)
		}
		delete(want, capability.ID)
	}
	for id := range want {
		t.Errorf("exact passing row did not publish %q: %+v", id, document.Capabilities)
	}

	document, err = generator.Generate(facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "24.04", Arch: types.X86, Init: facts.InitOpenRC,
	}, "v1")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"resource:service", "provider:init/systemd", "provider:init/openrc"} {
		if _, found := capabilityWithID(document.Capabilities, id); found {
			t.Errorf("mismatched runtime fact published %q without an applicable exact row: %+v", id, document.Capabilities)
		}
	}
}

func capabilityWithID(capabilities []Capability, id string) (Capability, bool) {
	for _, capability := range capabilities {
		if capability.ID == id {
			return capability, true
		}
	}
	return Capability{}, false
}

func TestGeneratorPublishesQualifiedPacmanAURRepositoryAndTrustFeaturesIndependently(t *testing.T) {
	matrix := providermatrix.Matrix{Version: 1, Dependencies: providermatrix.AcceptedDependencyGates(), Rows: []providermatrix.Row{
		{CapabilityID: "package", Provider: "package", Distribution: "arch", Release: "2026-07-06", Architecture: "amd64", Backend: "pacman", ContractRevision: "v1", Environment: "container", Status: "passing", Selectors: []string{"make:provider-matrix-pacman-arch-2026-07-06"}},
		{CapabilityID: "package", Provider: "package", Distribution: "arch", Release: "2026-07-06", Architecture: "amd64", Backend: "yay", ContractRevision: "v1", Environment: "container", Status: "passing", Selectors: []string{"make:provider-matrix-aur-arch-2026-07-06"}},
		{CapabilityID: "repository", Provider: "repository", Distribution: "arch", Release: "2026-07-06", Architecture: "amd64", Backend: "pacman", ContractRevision: "v1", Environment: "container", Status: "passing", Selectors: []string{"make:provider-matrix-pacman-repository-arch-2026-07-06"}},
	}}
	generator, err := NewDefaultGeneratorWithProviderMatrix([]int{1}, matrix)
	if err != nil {
		t.Fatal(err)
	}
	document, err := generator.Generate(facts.Facts{Distro: types.Arch, DistroVersion: "2026-07-06", Arch: types.X86, Package: types.Pacman}, "v1")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"provider:package/pacman":    {"lifecycle:present", "version:exact", "policy:downgrade"},
		"provider:package/yay":       {"aur:unprivileged-build", "aur:exact-artifact-install", "aur:artifact-digest"},
		"provider:repository/pacman": {"repository:present", "repository:disabled", "repository:signature-policy"},
		"provider:trust/pacman":      {"trust:full-fingerprint", "trust:provider-native-local-sign"},
	}
	for id, features := range want {
		capability, found := capabilityWithID(document.Capabilities, id)
		if !found {
			t.Fatalf("missing capability %q: %+v", id, document.Capabilities)
		}
		for _, feature := range features {
			if !slices.Contains(capability.Features, feature) {
				t.Errorf("capability %q features = %v, missing %q", id, capability.Features, feature)
			}
		}
	}

	document, err = generator.Generate(facts.Facts{Distro: types.Arch, DistroVersion: "2026-07-07", Arch: types.X86, Package: types.Pacman}, "v1")
	if err != nil {
		t.Fatal(err)
	}
	for id := range want {
		if _, found := capabilityWithID(document.Capabilities, id); found {
			t.Errorf("mismatched release advertised %q", id)
		}
	}
}

func TestDefaultGeneratorAdvertisesOnlyQualifiedCoreRows(t *testing.T) {
	generator, err := NewDefaultGenerator([]int{1})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		endpoint facts.Facts
		want     []string
	}{
		{facts.Facts{Distro: types.Debian, DistroVersion: "12", Arch: types.X86, Package: types.Apt}, []string{"provider:package/apt", "provider:repository/apt", "provider:trust/apt"}},
		{facts.Facts{Distro: types.Ubuntu, DistroVersion: "24.04", Arch: types.X86, Package: types.Apt}, []string{"provider:package/apt", "provider:repository/apt", "provider:trust/apt"}},
		{facts.Facts{Distro: types.Arch, DistroVersion: "2026-07-06", Arch: types.X86, Package: types.Pacman}, []string{"provider:package/pacman", "provider:package/yay", "provider:repository/pacman", "provider:trust/pacman"}},
	} {
		document, err := generator.Generate(test.endpoint, "v1")
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range test.want {
			if _, found := capabilityWithID(document.Capabilities, id); !found {
				t.Errorf("qualified endpoint %+v omitted %q", test.endpoint, id)
			}
		}
		for _, id := range []string{"provider:package/dnf", "provider:package/rpm", "provider:package/apk", "provider:package/zypper", "provider:package/snap"} {
			if _, found := capabilityWithID(document.Capabilities, id); found {
				t.Errorf("qualified endpoint %+v advertised deferred %q", test.endpoint, id)
			}
		}
	}
}

// OS-AEC-093, OS-AEC-094, OS-AEC-097: completed Ubuntu evidence promotes
// only its exact rows. Endpoint facts cannot broaden that result into access,
// service, network, or other unqualified resource claims.
func TestDefaultGeneratorAdvertisesOnlyQualifiedUbuntuRows(t *testing.T) {
	generator, err := NewDefaultGenerator([]int{1})
	if err != nil {
		t.Fatal(err)
	}
	document, err := generator.Generate(facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "24.04", Arch: types.X86,
		Init: facts.InitSystemd, Package: types.Apt, Firewall: facts.FirewallNftables,
		Network: facts.NetworkManager, Security: facts.SecurityAppArmor,
	}, "v1")
	if err != nil {
		t.Fatal(err)
	}
	for id, revision := range map[string]string{
		"resource:file": "file-v1", "resource:download": "download-v1", "resource:directory": "directory-v1",
		"resource:link": "link-v1", "resource:group": "group-v1", "resource:user": "user-v1",
		"resource:authorized-key": "authorizedKey-v1",
	} {
		capability, found := capabilityWithID(document.Capabilities, id)
		if !found || capability.Revision != revision {
			t.Errorf("exact qualified Ubuntu capability %s/%s is absent: %+v", id, revision, document.Capabilities)
		}
	}
	for _, id := range []string{
		"resource:sudo", "resource:user-file", "resource:service",
		"resource:sysctl", "resource:firewall", "resource:certificate",
	} {
		if _, found := capabilityWithID(document.Capabilities, id); found {
			t.Errorf("qualified filesystem and identity evidence broadened into %q: %+v", id, document.Capabilities)
		}
	}
}
