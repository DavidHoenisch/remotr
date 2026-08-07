package capabilitydoc

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/providermatrix"
	"github.com/DavidHoenisch/remotr/internal/types"
	"github.com/DavidHoenisch/remotr/internal/ubuntuproqualification"
)

func TestGeneratorPublishesOnlyExactPassingUbuntuProRelease(t *testing.T) {
	qualification, err := ubuntuproqualification.Load(filepath.Join("..", "..", "test", "qualification", "ubuntu-pro.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	qualification = qualification.Clone()
	for index := range qualification.BaseRows {
		if qualification.BaseRows[index].Release == "26.04" {
			qualification.BaseRows[index].Status = "passing"
		}
	}
	matrix := providermatrix.Matrix{Version: 1, Dependencies: providermatrix.AcceptedDependencyGates(), Rows: []providermatrix.Row{{
		CapabilityID: "file", Provider: "filesystem", Distribution: "ubuntu", Release: "24.04", Architecture: "amd64",
		Backend: "posix", ContractRevision: "file-v1", Environment: "container", Status: "passing",
		Selectors: []string{"go-test:./internal/capabilitydoc:^TestGeneratorPublishesOnlyExactPassingUbuntuProRelease$"},
	}}}
	generator, err := NewDefaultGeneratorWithUbuntuProQualification([]int{1}, matrix, qualification)
	if err != nil {
		t.Fatal(err)
	}

	exact := facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86,
		OSID: "ubuntu", OSReleaseSourceCount: 2, OSReleaseConsistent: true, DistroVendor: "Ubuntu",
	}
	document, err := generator.Generate(exact, "v1")
	if err != nil {
		t.Fatal(err)
	}
	if capability, ok := capabilityWithID(document.Capabilities, "resource:ubuntu-pro"); !ok || capability.Revision != "ubuntu-pro-v1" {
		t.Fatalf("Ubuntu Pro capability = %+v, found=%t; document=%+v", capability, ok, document.Capabilities)
	}
	for _, present := range []string{
		"provider:ubuntu-pro-service/esm-infra",
		"provider:ubuntu-pro-option/esm-infra/full",
		"provider:ubuntu-pro-variant/realtime-kernel/raspi",
	} {
		if _, ok := capabilityWithID(document.Capabilities, present); !ok {
			t.Errorf("26.04 omitted independently passing capability %q: %+v", present, document.Capabilities)
		}
	}
	for _, absent := range []string{
		"resource:file",
		"provider:ubuntu-pro-option/esm-infra/access-only",
		"provider:ubuntu-pro-disable/esm-infra/purge",
	} {
		if _, ok := capabilityWithID(document.Capabilities, absent); ok {
			t.Errorf("26.04 uplifted unproven capability %q: %+v", absent, document.Capabilities)
		}
	}

	for _, endpoint := range []facts.Facts{
		{Distro: types.Ubuntu, DistroVersion: "25.10", Arch: types.X86, OSID: "ubuntu", OSReleaseConsistent: true, DistroVendor: "Ubuntu"},
		{Distro: types.Ubuntu, DistroVersion: "28.04", Arch: types.X86, OSID: "ubuntu", OSReleaseConsistent: true, DistroVendor: "Ubuntu"},
		{Distro: types.PopOS, DistroFamily: facts.DistroFamilyDebian, DistroVersion: "26.04", Arch: types.X86, OSID: "pop", OSIDLike: []string{"ubuntu", "debian"}, OSReleaseConsistent: true, DistroVendor: "Ubuntu"},
	} {
		document, err := generator.Generate(endpoint, "v1")
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := capabilityWithID(document.Capabilities, "resource:ubuntu-pro"); ok {
			t.Errorf("inexact endpoint %+v advertised Ubuntu Pro: %+v", endpoint, document.Capabilities)
		}
	}
}

// OS-LPC-011 and OS-LPC-028. Public seam: production capability document
// generation used by authenticated Sync.
func TestGeneratorPreservesPopOSIdentityWithoutInheritingDebianQualification(t *testing.T) {
	matrix := providermatrix.Matrix{Version: 1, Dependencies: providermatrix.AcceptedDependencyGates(), Rows: []providermatrix.Row{{
		CapabilityID: "flatpak", Provider: "flatpak", Distribution: "debian", Release: "24.04", Architecture: "amd64",
		Backend: "flatpak", ContractRevision: "v1", Environment: "vm", Status: "passing",
		Selectors: []string{"go-test:./internal/capabilitydoc:^TestGeneratorPreservesPopOSIdentityWithoutInheritingDebianQualification$"},
	}}}
	generator, err := NewDefaultGeneratorWithProviderMatrix([]int{1}, matrix)
	if err != nil {
		t.Fatal(err)
	}
	document, err := generator.Generate(facts.Facts{
		Distro: types.PopOS, DistroFamily: facts.DistroFamilyDebian,
		DistroVersion: "24.04", Arch: types.X86, Package: types.Apt,
		UniversalPackage: []types.PackageManager{types.Flatpak},
		OSID:             "pop", OSIDLike: []string{"ubuntu", "debian"}, OSReleaseConsistent: true,
	}, "v1")
	if err != nil {
		t.Fatal(err)
	}
	wantFacts := map[string]string{"distro": "popos", "distro-family": "debian"}
	for key, want := range wantFacts {
		found := false
		for _, fact := range document.Facts {
			if fact.Key == key && fact.Value == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("capability facts omit %s=%s: %+v", key, want, document.Facts)
		}
	}
	for _, inherited := range []string{"provider:package/flatpak", "resource:package", "resource:ubuntu-pro"} {
		if _, ok := capabilityWithID(document.Capabilities, inherited); ok {
			t.Errorf("Pop!_OS inherited unqualified capability %q: %+v", inherited, document.Capabilities)
		}
	}
}

// OS-LPC-023 and OS-UPM-061. Public seam: production capability document
// generation used by composed agent Sync. Passing release evidence must not
// require the test-only injection constructor.
func TestDefaultGeneratorIncludesFrozenUbuntuProQualification(t *testing.T) {
	generator, err := NewDefaultGenerator([]int{1})
	if err != nil {
		t.Fatal(err)
	}
	document, err := generator.Generate(facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86, Package: types.Apt,
		OSID: "ubuntu", OSReleaseSourceCount: 2, OSReleaseConsistent: true, DistroVendor: "Ubuntu",
	}, "v0.6.8")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []Capability{
		{ID: "resource:ubuntu-pro", Revision: "ubuntu-pro-v1"},
		{ID: "provider:ubuntu-pro-service/esm-apps", Revision: "1"},
		{ID: "provider:ubuntu-pro-option/esm-apps/full", Revision: "1"},
	} {
		got, ok := capabilityWithID(document.Capabilities, expected.ID)
		if !ok || got.Revision != expected.Revision {
			t.Fatalf("default production capabilities omit %+v: %+v", expected, document.Capabilities)
		}
	}

	qualification, err := ubuntuproqualification.Load(filepath.Join("..", "..", "test", "qualification", "ubuntu-pro.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	wantCatalog := qualification.AdvertisedCapabilities(ubuntuproqualification.Target{
		Distribution: "ubuntu", Release: "26.04", Architecture: "amd64", APIRevision: "ubuntu-pro-api-v32",
	})
	for _, capability := range document.Capabilities {
		if strings.HasPrefix(capability.ID, "provider:ubuntu-pro-") || capability.ID == "resource:ubuntu-pro" {
			if !slices.Contains(wantCatalog, capability.ID) {
				t.Errorf("production agent advertised capability absent from frozen catalog: %q", capability.ID)
			}
		}
	}

	for name, endpoint := range map[string]facts.Facts{
		"mismatched release": {Distro: types.Ubuntu, DistroVersion: "25.10", Arch: types.X86, OSID: "ubuntu", OSReleaseConsistent: true, DistroVendor: "Ubuntu"},
		"architecture":       {Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.Arm, OSID: "ubuntu", OSReleaseConsistent: true, DistroVendor: "Ubuntu"},
		"derivative":         {Distro: types.PopOS, DistroFamily: facts.DistroFamilyDebian, DistroVersion: "26.04", Arch: types.X86, OSID: "pop", OSIDLike: []string{"ubuntu"}, OSReleaseConsistent: true, DistroVendor: "Ubuntu"},
		"backend identity":   {Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86, OSID: "ubuntu", OSReleaseConsistent: true, DistroVendor: "Debian"},
		"incomplete facts":   {Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86, OSID: "ubuntu", OSReleaseConsistent: false, DistroVendor: "Ubuntu"},
	} {
		document, err := generator.Generate(endpoint, "v0.6.8")
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := capabilityWithID(document.Capabilities, "resource:ubuntu-pro"); ok {
			t.Errorf("%s advertised Ubuntu Pro: %+v", name, document.Capabilities)
		}
	}
	withoutObservedPortableProviders, err := generator.Generate(facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86,
		Init: facts.InitSystemd, Package: types.Apt,
	}, "v0.6.8")
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"provider:package/flatpak", "provider:package/pwa"} {
		if _, ok := capabilityWithID(withoutObservedPortableProviders.Capabilities, absent); ok {
			t.Errorf("unobserved portable provider advertised %q: %+v", absent, withoutObservedPortableProviders.Capabilities)
		}
	}
	chromeDocument, err := generator.Generate(facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86,
		Init: facts.InitSystemd, Package: types.Apt,
		Browser: []facts.BrowserBackend{facts.BrowserGoogleChrome},
	}, "v0.6.8")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := capabilityWithID(chromeDocument.Capabilities, "provider:package/pwa"); !ok {
		t.Errorf("qualified Google Chrome backend omitted PWA: %+v", chromeDocument.Capabilities)
	}
}

// OS-LPC-027. Public seam: the production capability document generated from
// exact endpoint facts. Only provider contracts with pinned Ubuntu 26.04 VM
// evidence may be advertised.
func TestDefaultGeneratorPublishesQualifiedUbuntu2604CoreDelivery(t *testing.T) {
	generator, err := NewDefaultGenerator([]int{1})
	if err != nil {
		t.Fatal(err)
	}
	document, err := generator.Generate(facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86,
		Init: facts.InitSystemd, Package: types.Apt,
		UniversalPackage: []types.PackageManager{types.Flatpak},
		Browser:          []facts.BrowserBackend{facts.BrowserChromium},
	}, "v0.6.8")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []Capability{
		{ID: "resource:bootstrap", Revision: "bootstrap-v1"},
		{ID: "resource:command", Revision: "command-v1"},
		{ID: "resource:systemd", Revision: "systemd-v1"},
		{ID: "provider:init/systemd", Revision: "1"},
		{ID: "provider:package/flatpak", Revision: "1"},
		{ID: "provider:package/pwa", Revision: "1"},
	} {
		got, ok := capabilityWithID(document.Capabilities, expected.ID)
		if !ok || got.Revision != expected.Revision {
			t.Errorf("production capabilities omit %+v: %+v", expected, document.Capabilities)
		}
	}
}

// OS-LPC-034 and OS-LPC-035. Public seam: production capability document from
// exact endpoint facts. Container applicator rows proven on Ubuntu 24.04 are
// advertised on Ubuntu 26.04 only after exact 26.04 evidence; unproved
// applicators remain fail-closed on the incomplete release side.
func TestDefaultGeneratorPublishesQualifiedUbuntu2604ContainerApplicators(t *testing.T) {
	generator, err := NewDefaultGenerator([]int{1})
	if err != nil {
		t.Fatal(err)
	}
	document, err := generator.Generate(facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86,
		Init: facts.InitSystemd, Package: types.Apt,
	}, "v0.6.23")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []Capability{
		{ID: "resource:directory", Revision: "directory-v1"},
		{ID: "resource:link", Revision: "link-v1"},
		{ID: "resource:known-host", Revision: "knownHost-v1"},
		{ID: "resource:endpoint-schedule", Revision: "endpointSchedule-v1"},
		{ID: "provider:schedule/cron", Revision: "1"},
	} {
		got, ok := capabilityWithID(document.Capabilities, expected.ID)
		if !ok || got.Revision != expected.Revision {
			t.Errorf("production capabilities omit %+v: %+v", expected, document.Capabilities)
		}
	}
	for _, absent := range []string{"resource:desktop-setting"} {
		if _, ok := capabilityWithID(document.Capabilities, absent); ok {
			t.Errorf("incomplete Ubuntu 26.04 applicator side advertised %s: %+v", absent, document.Capabilities)
		}
	}
	for name, target := range map[string]facts.Facts{
		"another architecture": {
			Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.Arm,
			Init: facts.InitSystemd, Package: types.Apt,
		},
		"another release": {
			Distro: types.Ubuntu, DistroVersion: "22.04", Arch: types.X86,
			Init: facts.InitSystemd, Package: types.Apt,
		},
	} {
		t.Run(name, func(t *testing.T) {
			document, err := generator.Generate(target, "v0.6.23")
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{
				"resource:directory", "resource:link", "resource:known-host",
				"resource:endpoint-schedule", "provider:schedule/cron",
			} {
				if _, ok := capabilityWithID(document.Capabilities, id); ok {
					t.Errorf("Ubuntu 26.04 container applicator qualification broadened %s into %s", id, name)
				}
			}
		})
	}
}

// OS-LPC-034. Public seam: production capability document from exact endpoint
// facts. Ubuntu 26.04 host/system applicator rows advertise only after exact
// release evidence; unproved desktop/network applicators stay fail-closed.
func TestDefaultGeneratorPublishesQualifiedUbuntu2604HostSystemApplicators(t *testing.T) {
	generator, err := NewDefaultGenerator([]int{1})
	if err != nil {
		t.Fatal(err)
	}
	document, err := generator.Generate(facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86,
		Init: facts.InitSystemd, Package: types.Apt,
		Security: facts.SecurityAppArmor,
	}, "v0.6.23")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []Capability{
		{ID: "resource:sysctl", Revision: "sysctl-v1"},
		{ID: "provider:kernel/sysctl", Revision: "1"},
		{ID: "resource:kernel-module", Revision: "kernelModule-v1"},
		{ID: "provider:kernel/modules", Revision: "1"},
		{ID: "resource:hostname", Revision: "hostname-v1"},
		{ID: "provider:host/hostnamectl", Revision: "1"},
		{ID: "resource:host-locale", Revision: "hostLocale-v1"},
		{ID: "provider:host/localectl", Revision: "1"},
		{ID: "resource:time-sync", Revision: "timeSync-v1"},
		{ID: "provider:time-sync/systemd-timesyncd", Revision: "1"},
		{ID: "resource:mount", Revision: "mount-v1"},
		{ID: "provider:storage/mount", Revision: "1"},
		{ID: "resource:journald", Revision: "journald-v1"},
		{ID: "resource:logrotate", Revision: "logrotate-v1"},
		{ID: "provider:logging/logrotate", Revision: "1"},
		{ID: "resource:certificate", Revision: "certificate-v1"},
		{ID: "resource:trust-anchor", Revision: "trustAnchor-v1"},
		{ID: "resource:app-armor-profile", Revision: "appArmorProfile-v1"},
		{ID: "provider:security/apparmor", Revision: "1"},
		{ID: "resource:audit-rules", Revision: "auditRules-v1"},
		{ID: "resource:reboot", Revision: "reboot-v1"},
		{ID: "resource:systemd-unit", Revision: "systemdUnit-v1"},
		{ID: "resource:service", Revision: "service-state-v1"},
		{ID: "provider:schedule/systemd-timer", Revision: "1"},
	} {
		got, ok := capabilityWithID(document.Capabilities, expected.ID)
		if !ok || got.Revision != expected.Revision {
			t.Errorf("production capabilities omit %+v: %+v", expected, document.Capabilities)
		}
	}
	for _, absent := range []string{
		"resource:swap", "provider:storage/swap",
	} {
		if _, ok := capabilityWithID(document.Capabilities, absent); ok {
			t.Errorf("incomplete Ubuntu 26.04 host/system side advertised %s: %+v", absent, document.Capabilities)
		}
	}
}

// OS-LPC-034 and OS-LPC-035. Public seam: production capability document from
// exact endpoint facts. User/auth applicator rows proven on Ubuntu 24.04 are
// advertised on Ubuntu 26.04 only after exact 26.04 VM evidence.
func TestDefaultGeneratorPublishesQualifiedUbuntu2604UserAuthApplicators(t *testing.T) {
	generator, err := NewDefaultGenerator([]int{1})
	if err != nil {
		t.Fatal(err)
	}
	document, err := generator.Generate(facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86,
		Init: facts.InitSystemd, Package: types.Apt,
	}, "v0.6.23")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []Capability{
		{ID: "resource:group", Revision: "group-v1"},
		{ID: "resource:user", Revision: "user-v1"},
		{ID: "resource:authorized-key", Revision: "authorizedKey-v1"},
		{ID: "resource:sudo", Revision: "sudo-v1"},
		{ID: "resource:user-file", Revision: "userFile-v1"},
		{ID: "resource:account-limit", Revision: "accountLimit-v1"},
		{ID: "resource:login-policy", Revision: "loginPolicy-v1"},
	} {
		got, ok := capabilityWithID(document.Capabilities, expected.ID)
		if !ok || got.Revision != expected.Revision {
			t.Errorf("production capabilities omit %+v: %+v", expected, document.Capabilities)
		}
	}
	for name, target := range map[string]facts.Facts{
		"another architecture": {
			Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.Arm,
			Init: facts.InitSystemd, Package: types.Apt,
		},
		"another release": {
			Distro: types.Ubuntu, DistroVersion: "22.04", Arch: types.X86,
			Init: facts.InitSystemd, Package: types.Apt,
		},
	} {
		t.Run(name, func(t *testing.T) {
			document, err := generator.Generate(target, "v0.6.23")
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{
				"resource:group", "resource:user", "resource:authorized-key",
				"resource:sudo", "resource:user-file", "resource:account-limit",
				"resource:login-policy",
			} {
				if _, ok := capabilityWithID(document.Capabilities, id); ok {
					t.Errorf("Ubuntu 26.04 user/auth qualification broadened %s into %s", id, name)
				}
			}
		})
	}
}

// OS-LPC-034 and OS-LPC-035. Public seam: production capability document from
// exact endpoint facts. Network applicator rows proven on Ubuntu 24.04 are
// advertised on Ubuntu 26.04 only after exact 26.04 VM evidence.
func TestDefaultGeneratorPublishesQualifiedUbuntu2604NetworkApplicators(t *testing.T) {
	generator, err := NewDefaultGenerator([]int{1})
	if err != nil {
		t.Fatal(err)
	}
	document, err := generator.Generate(facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86,
		Init: facts.InitSystemd, Package: types.Apt,
		Firewall: facts.FirewallNftables, Network: facts.NetworkManager,
	}, "v0.6.23")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []Capability{
		{ID: "resource:hosts-entry", Revision: "hostsEntry-v1"},
		{ID: "resource:dns-resolver", Revision: "dnsResolver-v1"},
		{ID: "resource:route", Revision: "route-v1"},
		{ID: "resource:network-profile", Revision: "networkProfile-v1"},
		{ID: "resource:firewall", Revision: "firewall-v1"},
	} {
		got, ok := capabilityWithID(document.Capabilities, expected.ID)
		if !ok || got.Revision != expected.Revision {
			t.Errorf("production capabilities omit %+v: %+v", expected, document.Capabilities)
		}
	}
	for _, absent := range []string{"resource:desktop-setting", "resource:session-policy", "resource:browser-policy"} {
		if _, ok := capabilityWithID(document.Capabilities, absent); ok {
			t.Errorf("incomplete Ubuntu 26.04 network side advertised %s: %+v", absent, document.Capabilities)
		}
	}
	for name, target := range map[string]facts.Facts{
		"another architecture": {
			Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.Arm,
			Init: facts.InitSystemd, Package: types.Apt,
			Firewall: facts.FirewallNftables, Network: facts.NetworkManager,
		},
		"another release": {
			Distro: types.Ubuntu, DistroVersion: "22.04", Arch: types.X86,
			Init: facts.InitSystemd, Package: types.Apt,
			Firewall: facts.FirewallNftables, Network: facts.NetworkManager,
		},
	} {
		t.Run(name, func(t *testing.T) {
			document, err := generator.Generate(target, "v0.6.23")
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{
				"resource:hosts-entry", "resource:dns-resolver", "resource:route",
				"resource:network-profile", "resource:firewall",
			} {
				if _, ok := capabilityWithID(document.Capabilities, id); ok {
					t.Errorf("Ubuntu 26.04 network qualification broadened %s into %s", id, name)
				}
			}
		})
	}
}

// OS-LPC-034 and OS-LPC-035. Public seam: production capability document from
// exact endpoint facts. Desktop applicator rows proven on Ubuntu 24.04 are
// advertised on Ubuntu 26.04 only after exact 26.04 VM evidence.
func TestDefaultGeneratorPublishesQualifiedUbuntu2604DesktopApplicators(t *testing.T) {
	generator, err := NewDefaultGenerator([]int{1})
	if err != nil {
		t.Fatal(err)
	}
	document, err := generator.Generate(facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.X86,
		Init: facts.InitSystemd, Package: types.Apt,
		Desktop: []facts.DesktopBackend{facts.DesktopDconf, facts.DesktopGSettings},
		Browser: []facts.BrowserBackend{facts.BrowserChromium, facts.BrowserGoogleChrome, facts.BrowserFirefox},
	}, "v0.6.23")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []Capability{
		{ID: "resource:desktop-setting", Revision: "desktopSetting-v1"},
		{ID: "resource:session-policy", Revision: "sessionPolicy-v1"},
		{ID: "resource:browser-policy", Revision: "browserPolicy-v1"},
		{ID: "provider:desktop/dconf", Revision: "1"},
		{ID: "provider:desktop/gsettings", Revision: "1"},
		{ID: "provider:browser/chromium", Revision: "1"},
		{ID: "provider:browser/google-chrome", Revision: "1"},
		{ID: "provider:browser/firefox", Revision: "1"},
	} {
		got, ok := capabilityWithID(document.Capabilities, expected.ID)
		if !ok || got.Revision != expected.Revision {
			t.Errorf("production capabilities omit %+v: %+v", expected, document.Capabilities)
		}
	}
	for name, target := range map[string]facts.Facts{
		"another architecture": {
			Distro: types.Ubuntu, DistroVersion: "26.04", Arch: types.Arm,
			Init: facts.InitSystemd, Package: types.Apt,
			Desktop: []facts.DesktopBackend{facts.DesktopDconf, facts.DesktopGSettings},
			Browser: []facts.BrowserBackend{facts.BrowserChromium, facts.BrowserGoogleChrome, facts.BrowserFirefox},
		},
		"another release": {
			Distro: types.Ubuntu, DistroVersion: "22.04", Arch: types.X86,
			Init: facts.InitSystemd, Package: types.Apt,
			Desktop: []facts.DesktopBackend{facts.DesktopDconf, facts.DesktopGSettings},
			Browser: []facts.BrowserBackend{facts.BrowserChromium, facts.BrowserGoogleChrome, facts.BrowserFirefox},
		},
	} {
		t.Run(name, func(t *testing.T) {
			document, err := generator.Generate(target, "v0.6.23")
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{
				"resource:desktop-setting", "resource:session-policy", "resource:browser-policy",
			} {
				if _, ok := capabilityWithID(document.Capabilities, id); ok {
					t.Errorf("Ubuntu 26.04 desktop qualification broadened %s into %s", id, name)
				}
			}
		})
	}
}

// OS-LPC-034 and OS-LPC-035. Public seam: production capability documents for
// equivalent Ubuntu 24.04 and 26.04 observed facts advertise the same non-Pro
// capability set. Deferred Ubuntu 26.04 swap remains fail-closed and is
// excluded from the equality assertion until exact VM evidence lands.
func TestDefaultGeneratorPublishesEqualUbuntu2404And2604NonProUnion(t *testing.T) {
	generator, err := NewDefaultGenerator([]int{1})
	if err != nil {
		t.Fatal(err)
	}
	base := facts.Facts{
		Distro: types.Ubuntu, Arch: types.X86,
		Init: facts.InitSystemd, Package: types.Apt,
		Firewall: facts.FirewallNftables, Network: facts.NetworkManager,
		Security:         facts.SecurityAppArmor,
		UniversalPackage: []types.PackageManager{types.Flatpak},
		Desktop:          []facts.DesktopBackend{facts.DesktopDconf, facts.DesktopGSettings},
		Browser:          []facts.BrowserBackend{facts.BrowserChromium, facts.BrowserGoogleChrome, facts.BrowserFirefox},
	}
	ubuntu2404 := base
	ubuntu2404.DistroVersion = "24.04"
	ubuntu2604 := base
	ubuntu2604.DistroVersion = "26.04"

	doc2404, err := generator.Generate(ubuntu2404, "v0.6.23")
	if err != nil {
		t.Fatal(err)
	}
	doc2604, err := generator.Generate(ubuntu2604, "v0.6.23")
	if err != nil {
		t.Fatal(err)
	}

	set2404 := nonProUnionCapabilityIDs(doc2404.Capabilities)
	set2604 := nonProUnionCapabilityIDs(doc2604.Capabilities)
	if !mapsEqual(set2404, set2604) {
		t.Fatalf("Ubuntu non-Pro capability union diverged\nonly-24.04=%v\nonly-26.04=%v",
			sortedMissing(set2404, set2604), sortedMissing(set2604, set2404))
	}
	for _, deferred := range []string{"resource:swap", "provider:storage/swap"} {
		if _, ok := capabilityWithID(doc2604.Capabilities, deferred); ok {
			t.Errorf("deferred Ubuntu 26.04 swap still advertised as %s", deferred)
		}
		if _, ok := capabilityWithID(doc2404.Capabilities, deferred); !ok {
			t.Errorf("Ubuntu 24.04 omitted proved %s while 26.04 stays deferred", deferred)
		}
	}
}

func nonProUnionCapabilityIDs(capabilities []Capability) map[string]string {
	out := make(map[string]string, len(capabilities))
	for _, capability := range capabilities {
		if strings.HasPrefix(capability.ID, "provider:ubuntu-pro-") || capability.ID == "resource:ubuntu-pro" {
			continue
		}
		switch capability.ID {
		case "resource:swap", "provider:storage/swap":
			continue
		}
		out[capability.ID] = capability.Revision
	}
	return out
}

func mapsEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func sortedMissing(have, want map[string]string) []string {
	missing := make([]string, 0)
	for key, value := range have {
		if want[key] != value {
			missing = append(missing, key+"@"+value)
		}
	}
	slices.Sort(missing)
	return missing
}

// OS-LPC-029 and OS-LPC-033. Public seam: the production capability document
// generated from exact endpoint facts. Core delivery and portable package
// contracts with pinned Ubuntu 24.04 VM evidence are advertised to enrolled
// Ubuntu 24.04 endpoints without manufacturing them for other releases/arches.
func TestDefaultGeneratorPublishesQualifiedUbuntu2404CoreDelivery(t *testing.T) {
	generator, err := NewDefaultGenerator([]int{1})
	if err != nil {
		t.Fatal(err)
	}
	document, err := generator.Generate(facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "24.04", Arch: types.X86,
		Init: facts.InitSystemd, Package: types.Apt,
		UniversalPackage: []types.PackageManager{types.Flatpak},
		Browser:          []facts.BrowserBackend{facts.BrowserChromium},
	}, "v0.6.22")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []Capability{
		{ID: "resource:bootstrap", Revision: "bootstrap-v1"},
		{ID: "resource:command", Revision: "command-v1"},
		{ID: "resource:systemd", Revision: "systemd-v1"},
		{ID: "provider:package/flatpak", Revision: "1"},
		{ID: "provider:package/pwa", Revision: "1"},
	} {
		got, ok := capabilityWithID(document.Capabilities, expected.ID)
		if !ok || got.Revision != expected.Revision {
			t.Errorf("production capabilities omit %+v: %+v", expected, document.Capabilities)
		}
	}
	chromeDocument, err := generator.Generate(facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "24.04", Arch: types.X86,
		Init: facts.InitSystemd, Package: types.Apt,
		Browser: []facts.BrowserBackend{facts.BrowserGoogleChrome},
	}, "v0.6.22")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := capabilityWithID(chromeDocument.Capabilities, "provider:package/pwa"); !ok {
		t.Errorf("qualified Google Chrome backend omitted PWA: %+v", chromeDocument.Capabilities)
	}
	for name, target := range map[string]facts.Facts{
		"another architecture": {
			Distro: types.Ubuntu, DistroVersion: "24.04", Arch: types.Arm,
			Init: facts.InitSystemd, Package: types.Apt,
			UniversalPackage: []types.PackageManager{types.Flatpak},
			Browser:          []facts.BrowserBackend{facts.BrowserChromium},
		},
		"another release": {
			Distro: types.Ubuntu, DistroVersion: "22.04", Arch: types.X86,
			Init: facts.InitSystemd, Package: types.Apt,
			UniversalPackage: []types.PackageManager{types.Flatpak},
			Browser:          []facts.BrowserBackend{facts.BrowserChromium},
		},
	} {
		t.Run(name, func(t *testing.T) {
			document, err := generator.Generate(target, "v0.6.22")
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{
				"resource:bootstrap", "resource:command", "resource:systemd",
				"provider:package/flatpak", "provider:package/pwa",
			} {
				if _, ok := capabilityWithID(document.Capabilities, id); ok {
					t.Errorf("Ubuntu 24.04 amd64 qualification broadened %s into %s", id, name)
				}
			}
		})
	}
}

// OS-LPC-031 and OS-LPC-032. Public seam: production capability document
// generation used by composed agent Sync. Exact Pop!_OS 24.04 amd64 rows must
// advertise the unblock set without inheriting Ubuntu Pro or other releases.
func TestDefaultGeneratorPublishesQualifiedPopOS2404CoreDelivery(t *testing.T) {
	generator, err := NewDefaultGenerator([]int{1})
	if err != nil {
		t.Fatal(err)
	}
	document, err := generator.Generate(facts.Facts{
		Distro: types.PopOS, DistroFamily: facts.DistroFamilyDebian,
		DistroVersion: "24.04", Arch: types.X86,
		Init: facts.InitSystemd, Package: types.Apt,
		UniversalPackage: []types.PackageManager{types.Flatpak},
		Browser:          []facts.BrowserBackend{facts.BrowserChromium},
		OSID:             "pop", OSIDLike: []string{"ubuntu", "debian"}, OSReleaseConsistent: true,
	}, "v0.6.23")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []Capability{
		{ID: "provider:package/apt", Revision: "1"},
		{ID: "provider:init/systemd", Revision: "1"},
		{ID: "provider:package/flatpak", Revision: "1"},
		{ID: "provider:package/pwa", Revision: "1"},
		{ID: "resource:package", Revision: "package-v1"},
		{ID: "resource:file", Revision: "file-v1"},
		{ID: "resource:download", Revision: "download-v1"},
		{ID: "resource:bootstrap", Revision: "bootstrap-v1"},
		{ID: "resource:command", Revision: "command-v1"},
		{ID: "resource:systemd", Revision: "systemd-v1"},
	} {
		got, ok := capabilityWithID(document.Capabilities, expected.ID)
		if !ok || got.Revision != expected.Revision {
			t.Errorf("production capabilities omit %+v: %+v", expected, document.Capabilities)
		}
	}
	if _, ok := capabilityWithID(document.Capabilities, "resource:ubuntu-pro"); ok {
		t.Errorf("Pop!_OS advertised ubuntu-pro: %+v", document.Capabilities)
	}
	chromeDocument, err := generator.Generate(facts.Facts{
		Distro: types.PopOS, DistroFamily: facts.DistroFamilyDebian,
		DistroVersion: "24.04", Arch: types.X86,
		Init: facts.InitSystemd, Package: types.Apt,
		Browser: []facts.BrowserBackend{facts.BrowserGoogleChrome},
		OSID:    "pop", OSIDLike: []string{"ubuntu", "debian"}, OSReleaseConsistent: true,
	}, "v0.6.33")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := capabilityWithID(chromeDocument.Capabilities, "provider:package/pwa"); !ok {
		t.Errorf("qualified Pop!_OS Google Chrome backend omitted PWA: %+v", chromeDocument.Capabilities)
	}

	for _, blocked := range []facts.Facts{
		{
			Distro: types.PopOS, DistroFamily: facts.DistroFamilyDebian,
			DistroVersion: "22.04", Arch: types.X86,
			Init: facts.InitSystemd, Package: types.Apt,
			UniversalPackage: []types.PackageManager{types.Flatpak},
			Browser:          []facts.BrowserBackend{facts.BrowserChromium},
			OSID:             "pop", OSReleaseConsistent: true,
		},
		{
			Distro: types.PopOS, DistroFamily: facts.DistroFamilyDebian,
			DistroVersion: "24.04", Arch: types.Arm,
			Init: facts.InitSystemd, Package: types.Apt,
			UniversalPackage: []types.PackageManager{types.Flatpak},
			Browser:          []facts.BrowserBackend{facts.BrowserChromium},
			OSID:             "pop", OSReleaseConsistent: true,
		},
	} {
		blockedDoc, err := generator.Generate(blocked, "v0.6.23")
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{
			"provider:package/apt", "provider:init/systemd", "provider:package/flatpak", "provider:package/pwa",
			"resource:package", "resource:file", "resource:download",
			"resource:bootstrap", "resource:command", "resource:systemd",
		} {
			if _, ok := capabilityWithID(blockedDoc.Capabilities, id); ok {
				t.Errorf("unqualified Pop!_OS facts %+v advertised %s: %+v", blocked, id, blockedDoc.Capabilities)
			}
		}
	}
}

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

func TestGeneratorPublishesBuiltinRemotrPackageProvider(t *testing.T) {
	generator, err := NewDefaultGeneratorWithProviderMatrix([]int{1}, providermatrix.Matrix{Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	document, err := generator.Generate(facts.Facts{
		Distro: types.Debian, DistroVersion: "13", Arch: types.X86,
	}, "v1")
	if err != nil {
		t.Fatal(err)
	}

	capability, found := capabilityWithID(document.Capabilities, "provider:package/remotr")
	if !found || capability.Revision != "1" {
		t.Fatalf("built-in Remotr package provider = %+v, found=%t", capability, found)
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
		"provider:package/remotr": "1",
		"resource:file":           "file-v1", "resource:service": "service-state-v1", "resource:sysctl": "sysctl-v1",
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

// OS-AEC-093, OS-AEC-094, OS-AEC-097, OS-AEC-098: completed Ubuntu evidence promotes
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
		Desktop: []facts.DesktopBackend{facts.DesktopDconf, facts.DesktopGSettings},
		Browser: []facts.BrowserBackend{facts.BrowserChromium, facts.BrowserGoogleChrome, facts.BrowserFirefox},
	}, "v1")
	if err != nil {
		t.Fatal(err)
	}
	for id, revision := range map[string]string{
		"resource:file": "file-v1", "resource:download": "download-v1", "resource:directory": "directory-v1",
		"resource:link": "link-v1", "resource:group": "group-v1", "resource:user": "user-v1",
		"resource:authorized-key":              "authorizedKey-v1",
		"resource:known-host":                  "knownHost-v1",
		"resource:sudo":                        "sudo-v1",
		"resource:user-file":                   "userFile-v1",
		"resource:sysctl":                      "sysctl-v1",
		"provider:kernel/sysctl":               "1",
		"resource:kernel-module":               "kernelModule-v1",
		"provider:kernel/modules":              "1",
		"resource:hostname":                    "hostname-v1",
		"provider:host/hostnamectl":            "1",
		"resource:host-locale":                 "hostLocale-v1",
		"provider:host/localectl":              "1",
		"resource:time-sync":                   "timeSync-v1",
		"provider:time-sync/systemd-timesyncd": "1",
		"resource:mount":                       "mount-v1",
		"provider:storage/mount":               "1",
		"resource:swap":                        "swap-v1",
		"provider:storage/swap":                "1",
		"resource:endpoint-schedule":           "endpointSchedule-v1",
		"provider:schedule/cron":               "1",
		"provider:schedule/systemd-timer":      "1",
		"provider:init/systemd":                "1",
		"resource:service":                     "service-state-v1",
		"resource:systemd-unit":                "systemdUnit-v1",
		"resource:reboot":                      "reboot-v1",
		"resource:hosts-entry":                 "hostsEntry-v1",
		"resource:dns-resolver":                "dnsResolver-v1",
		"resource:route":                       "route-v1",
		"resource:network-profile":             "networkProfile-v1",
		"resource:firewall":                    "firewall-v1",
		"resource:certificate":                 "certificate-v1",
		"resource:trust-anchor":                "trustAnchor-v1",
		"resource:app-armor-profile":           "appArmorProfile-v1",
		"provider:security/apparmor":           "1",
		"resource:audit-rules":                 "auditRules-v1",
		"resource:account-limit":               "accountLimit-v1",
		"resource:login-policy":                "loginPolicy-v1",
		"resource:journald":                    "journald-v1",
		"resource:logrotate":                   "logrotate-v1",
		"resource:desktop-setting":             "desktopSetting-v1",
		"resource:session-policy":              "sessionPolicy-v1",
		"resource:browser-policy":              "browserPolicy-v1",
		"provider:desktop/dconf":               "1",
		"provider:desktop/gsettings":           "1",
		"provider:browser/chromium":            "1",
		"provider:browser/google-chrome":       "1",
		"provider:browser/firefox":             "1",
	} {
		capability, found := capabilityWithID(document.Capabilities, id)
		if !found || capability.Revision != revision {
			t.Errorf("exact qualified Ubuntu capability %s/%s is absent: %+v", id, revision, document.Capabilities)
		}
	}
	for _, id := range []string{"resource:systemdUser"} {
		if _, found := capabilityWithID(document.Capabilities, id); found {
			t.Errorf("qualified filesystem and identity evidence broadened into %q: %+v", id, document.Capabilities)
		}
	}
}
