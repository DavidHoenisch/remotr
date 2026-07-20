package capabilitymatrix

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/providermatrix"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// OS-TQG-012: capability selection is mutation-sensitive at the public static
// validation, requirement, and runtime-check seams.
func TestCapabilityMatrixCoversAdvertisedProviderBranches(t *testing.T) {
	t.Run("static distribution and provider restrictions", func(t *testing.T) {
		tests := []struct {
			name    string
			targets []types.Distro
			value   any
			wantErr bool
		}{
			{"APT key on Ubuntu", []types.Distro{types.Ubuntu}, &models.APTSigningKey{}, false},
			{"APT key on Arch", []types.Distro{types.Arch}, &models.APTSigningKey{}, true},
			{"login policy on Debian", []types.Distro{types.Debian}, &models.LoginPolicyResource{}, false},
			{"login policy on Arch", []types.Distro{types.Arch}, &models.LoginPolicyResource{}, true},
			{"portable package", []types.Distro{types.Arch}, &models.Package{PM: types.Flatpak}, false},
			{"unknown package provider", []types.Distro{types.Debian}, &models.Package{PM: types.PackageManager("unknown")}, true},
			{"known provider option", []types.Distro{types.Debian}, &models.Package{ResourceMeta: models.ResourceMeta{ProviderOptions: map[string]map[string]any{"apt": {}}}}, false},
			{"unknown provider option", []types.Distro{types.Debian}, &models.Package{ResourceMeta: models.ResourceMeta{ProviderOptions: map[string]map[string]any{"unknown": {}}}}, true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := ValidateStatic(1, models.Configuration{TargetDistros: tt.targets}, tt.value)
				if (err != nil) != tt.wantErr {
					t.Fatalf("ValidateStatic() err = %v, wantErr %t", err, tt.wantErr)
				}
			})
		}
	})

	t.Run("requirements retain selected providers", func(t *testing.T) {
		tests := []struct {
			kind  models.ResourceKind
			value any
			want  []string
		}{
			{models.ResourceKindPackage, &models.Package{PM: types.Apt}, []string{"provider:package/apt"}},
			{models.ResourceKindFirewall, &models.FirewallResource{Backend: " NFTABLES "}, []string{"provider:firewall/nftables"}},
			{models.ResourceKindEndpointSchedule, &models.EndpointScheduleResource{Backend: models.ScheduleBackendSystemdTimer}, []string{"provider:schedule/systemd-timer", "provider:init/systemd"}},
		}
		for _, tt := range tests {
			got := Requirements(tt.kind, tt.value)
			for _, want := range tt.want {
				if !slices.Contains(got, want) {
					t.Fatalf("Requirements(%s) = %v, missing %q", tt.kind, got, want)
				}
			}
		}
	})

	t.Run("runtime matches and typed mismatches", func(t *testing.T) {
		tests := []struct {
			name       string
			value      any
			endpoint   facts.Facts
			wantErr    bool
			capability string
		}{
			{"unspecified package", &models.Package{}, facts.Facts{}, false, ""},
			{"DNF remains unavailable", &models.Package{PM: types.Dnf}, facts.Facts{Package: types.Dnf}, true, "package"},
			{"Yay uses Pacman database", &models.Package{PM: types.Yay}, facts.Facts{Package: types.Pacman}, false, ""},
			{"APT matches", &models.Package{PM: types.Apt}, facts.Facts{Package: types.Apt}, false, ""},
			{"APT mismatches", &models.Package{PM: types.Apt}, facts.Facts{Package: types.Pacman}, true, "package"},
			{"systemd resource mismatch", &models.SystemdResource{}, facts.Facts{Init: facts.InitOpenRC}, true, "init"},
			{"systemd schedule mismatch", &models.EndpointScheduleResource{Backend: models.ScheduleBackendSystemdTimer}, facts.Facts{Init: facts.InitOpenRC}, true, "init"},
			{"APT repository mismatch", &models.APTRepository{}, facts.Facts{Package: types.Pacman}, true, "repository"},
			{"DNS mismatch", &models.DNSResolverResource{Provider: models.NetworkProviderNetworkManager}, facts.Facts{Network: facts.NetworkNetplan}, true, "network"},
			{"route mismatch", &models.RouteResource{Provider: models.NetworkProviderNetworkManager}, facts.Facts{Network: facts.NetworkSystemdNetwork}, true, "network"},
			{"login policy distro mismatch", &models.LoginPolicyResource{Provider: models.LoginPolicyPAMAuthUpdate}, facts.Facts{Distro: types.Arch}, true, "authentication"},
			{"desktop setting match", &models.DesktopSettingResource{Provider: models.DesktopSettingProviderDconf}, facts.Facts{Desktop: []facts.DesktopBackend{facts.DesktopGSettings, facts.DesktopDconf}}, false, ""},
			{"desktop setting mismatch", &models.DesktopSettingResource{Provider: models.DesktopSettingProviderDconf}, facts.Facts{Desktop: []facts.DesktopBackend{facts.DesktopGSettings}}, true, "desktop"},
			{"session policy match", &models.SessionPolicyResource{Provider: models.DesktopSettingProviderGSettings}, facts.Facts{Desktop: []facts.DesktopBackend{facts.DesktopDconf, facts.DesktopGSettings}}, false, ""},
			{"session policy mismatch", &models.SessionPolicyResource{Provider: models.DesktopSettingProviderGSettings}, facts.Facts{Desktop: []facts.DesktopBackend{facts.DesktopDconf}}, true, "desktop"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := CheckRuntime(tt.value, tt.endpoint)
				if !tt.wantErr {
					if err != nil {
						t.Fatal(err)
					}
					return
				}
				var unsupported UnsupportedError
				if !errors.As(err, &unsupported) || unsupported.Capability != tt.capability || !strings.Contains(err.Error(), "required "+tt.capability+" provider") {
					t.Fatalf("CheckRuntime() err = %T %v", err, err)
				}
			})
		}
	})
}

func TestRuntimeProviderQualificationFailsClosedOnMismatchedLocalDiscovery(t *testing.T) {
	row := providermatrix.Row{
		CapabilityID: "package", Provider: "package", Distribution: "debian", Release: "12", Architecture: "amd64", Backend: "apt",
		ContractRevision: "v1", Environment: "container", Status: "passing", Selectors: []string{"make:provider-matrix-apt-debian-12"},
	}
	matrix := providermatrix.Matrix{Version: 1, Rows: []providermatrix.Row{row}}
	resource := &models.Package{Name: "fixture", Present: true, PM: types.Apt}
	exact := facts.Facts{Distro: types.Debian, DistroVersion: "12", Arch: types.X86, Package: types.Apt}
	if err := CheckRuntimeWithProviderMatrix(resource, exact, matrix); err != nil {
		t.Fatalf("exact local discovery rejected: %v", err)
	}
	for name, mutate := range map[string]func(*facts.Facts){
		"distribution": func(value *facts.Facts) { value.Distro = types.Ubuntu },
		"release":      func(value *facts.Facts) { value.DistroVersion = "13" },
		"architecture": func(value *facts.Facts) { value.Arch = types.Arm },
		"backend":      func(value *facts.Facts) { value.Package = types.Pacman },
	} {
		t.Run(name, func(t *testing.T) {
			local := exact
			mutate(&local)
			if err := CheckRuntimeWithProviderMatrix(resource, local, matrix); err == nil {
				t.Fatalf("mismatched local discovery %+v was accepted", local)
			}
		})
	}
	if err := CheckRuntime(resource, exact); err != nil {
		t.Fatalf("repository-default qualified row was rejected by the agent runtime seam: %v", err)
	}
	mismatchedDefault := exact
	mismatchedDefault.DistroVersion = "13"
	if err := CheckRuntime(resource, mismatchedDefault); err == nil {
		t.Fatal("repository-default row accepted a mismatched release")
	}
}

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
