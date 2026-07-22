package resolve_test

import (
	"reflect"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/resolve"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestResolve_filtersByDistroAndArch(t *testing.T) {
	state := models.State{Configurations: []models.Configuration{
		{
			Name:          "debian-only",
			TargetDistros: []types.Distro{types.Debian},
			Packages:      []models.Package{{Name: "curl", Present: true, PM: types.Apt}},
		},
		{
			Name:          "arch-only",
			TargetDistros: []types.Distro{types.Arch},
			Packages:      []models.Package{{Name: "curl", Present: true, PM: types.Pacman}},
		},
		{
			Name:          "pop-only",
			TargetDistros: []types.Distro{types.PopOS},
			Files:         []models.File{{Name: "pop", Path: "/tmp/pop", Content: "managed"}},
		},
		{
			Name:       "x86-only",
			TargetArch: []types.Architecture{types.X86},
			Users:      []models.UserResource{{Name: "dev", Username: "dev", Present: true}},
		},
		{
			Name: "universal",
			Files: []models.File{
				{Name: "motd", Path: "/etc/motd", Content: "hello"},
			},
		},
	}}

	tests := []struct {
		name string
		f    facts.Facts
		want []string
	}{
		{
			name: "debian x86",
			f:    facts.Facts{Distro: types.Debian, Arch: types.X86},
			want: []string{"debian-only", "x86-only", "universal"},
		},
		{
			name: "arch arm",
			f:    facts.Facts{Distro: types.Arch, Arch: types.Arm},
			want: []string{"arch-only", "universal"},
		},
		{
			name: "pop x86",
			f:    facts.Facts{Distro: types.PopOS, DistroFamily: facts.DistroFamilyDebian, Arch: types.X86},
			want: []string{"pop-only", "x86-only", "universal"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolve.Resolve(state, tt.f)
			var names []string
			for _, c := range got.Configurations {
				names = append(names, c.Name)
			}
			if !reflect.DeepEqual(names, tt.want) {
				t.Fatalf("config names = %v, want %v", names, tt.want)
			}
		})
	}
}

func TestResolve_filtersPackagesByPM(t *testing.T) {
	state := models.State{Configurations: []models.Configuration{{
		Name:          "mixed",
		TargetDistros: []types.Distro{types.Debian, types.Arch},
		Packages: []models.Package{
			{Name: "curl", Present: true, PM: types.Apt},
			{Name: "curl", Present: true, PM: types.Pacman},
		},
	}}}

	got := resolve.Resolve(state, facts.Facts{Distro: types.Debian, Arch: types.X86})
	if len(got.Configurations) != 1 || len(got.Configurations[0].Packages) != 1 {
		t.Fatalf("expected one apt package, got %#v", got)
	}
	if got.Configurations[0].Packages[0].PM != types.Apt {
		t.Fatalf("expected apt package")
	}
}

func TestResolve_includesFlatpakOnAnyDistro(t *testing.T) {
	state := models.State{Configurations: []models.Configuration{{
		Name:          "flatpak-apps",
		TargetDistros: []types.Distro{types.Debian, types.Arch},
		Packages: []models.Package{
			{Name: "org.gnome.Calculator", Present: true, PM: types.Flatpak},
			{Name: "curl", Present: true, PM: types.Apt},
			{Name: "curl", Present: true, PM: types.Pacman},
		},
	}}}

	got := resolve.Resolve(state, facts.Facts{Distro: types.Arch, Arch: types.X86})
	if len(got.Configurations) != 1 || len(got.Configurations[0].Packages) != 2 {
		t.Fatalf("expected flatpak and pacman packages, got %#v", got)
	}
}

func TestResolve_includesPwaOnAnyDistro(t *testing.T) {
	state := models.State{Configurations: []models.Configuration{{
		Name:          "pwa-apps",
		TargetDistros: []types.Distro{types.Debian, types.Arch},
		Packages: []models.Package{
			{Name: "slack", Present: true, PM: types.Pwa, PWAURL: "https://app.slack.com/client"},
			{Name: "curl", Present: true, PM: types.Apt},
			{Name: "curl", Present: true, PM: types.Pacman},
		},
	}}}

	got := resolve.Resolve(state, facts.Facts{Distro: types.Arch, Arch: types.X86})
	if len(got.Configurations) != 1 || len(got.Configurations[0].Packages) != 2 {
		t.Fatalf("expected pwa and pacman packages, got %#v", got)
	}
}

func TestResolve_preservesEveryRegisteredResourceCollection(t *testing.T) {
	state := models.State{Configurations: []models.Configuration{{
		Name:            "all-kinds",
		Packages:        []models.Package{{Name: "package", Present: true, PM: types.Apt}},
		APTRepositories: []models.APTRepository{{Name: "repository", URL: "https://packages.example.test/debian", Suites: []string{"stable"}, Components: []string{"main"}, SigningKey: "vendor"}},
		Sysctls:         []models.SysctlResource{{Name: "forwarding", Key: "net.ipv4.ip_forward", Value: "1", Runtime: true}},
		Files:           []models.File{{Name: "file", Path: "/tmp/file"}},
		Directories:     []models.DirectoryResource{{Name: "directory", Path: "/tmp/directory", ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}}},
		Links:           []models.LinkResource{{Name: "link", Path: "/tmp/link", Target: "target", LinkType: models.LinkTypeSymbolic, ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}}},
		Groups:          []models.GroupResource{{Name: "group", Group: "example", ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}}},
		UserFiles:       []models.UserFileResource{{Name: "user-file", Users: "interactive", Path: ".config/file", Content: "managed\n"}},
		DesktopSettings: []models.DesktopSettingResource{{Name: "desktop-setting", Provider: models.DesktopSettingProviderDconf, Scope: models.DesktopSettingScopeUser, Selector: models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionAll}, Path: "/org/gnome/desktop/interface/enable-animations", Value: models.DesktopSettingValue{Type: models.DesktopValueBoolean, Value: false}}},
		SessionPolicies: []models.SessionPolicyResource{{Name: "session-policy", Provider: models.DesktopSettingProviderGSettings, Selector: models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionAll}, LockEnabled: boolValue(true)}},
		BrowserPolicies: []models.BrowserPolicyResource{{ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}, Name: "browser-policy", Browser: models.BrowserChromium, PolicyName: "HomepageLocation", Scope: models.BrowserPolicyScopeSystem, Level: models.BrowserPolicyLevelMandatory, Value: &models.BrowserPolicyValue{Type: models.BrowserValueString, Value: "https://example.test"}}},
		Downloads:       []models.DownloadResource{{Name: "download", URL: "https://example.com/file", Dest: "/tmp/download"}},
		Users:           []models.UserResource{{Name: "user", Username: "example", Present: true}},
		Systemd:         []models.SystemdResource{{Name: "systemd", Unit: "example.service"}},
		SystemdUser:     []models.SystemdUserResource{{Name: "systemd-user", Unit: "example.service", Users: "interactive"}},
		Bootstrap:       []models.BootstrapResource{{Name: "bootstrap"}},
		AgentInstall:    []models.AgentInstallResource{{Name: "agent-install"}},
		Firewall:        []models.FirewallResource{{Name: "firewall", Action: "allow"}},
		Commands:        []models.CommandResource{{Name: "command", Check: []string{"true"}}},
	}}}

	got := resolve.Resolve(state, facts.Facts{Distro: types.Debian, Arch: types.X86})
	if len(got.Configurations) != 1 {
		t.Fatalf("configurations = %d, want 1", len(got.Configurations))
	}
	cfg := got.Configurations[0]
	counts := map[string]int{
		"packages": len(cfg.Packages), "aptRepositories": len(cfg.APTRepositories), "sysctls": len(cfg.Sysctls), "files": len(cfg.Files), "directories": len(cfg.Directories), "links": len(cfg.Links), "groups": len(cfg.Groups), "userFiles": len(cfg.UserFiles), "desktopSettings": len(cfg.DesktopSettings), "sessionPolicies": len(cfg.SessionPolicies), "browserPolicies": len(cfg.BrowserPolicies),
		"downloads": len(cfg.Downloads), "users": len(cfg.Users), "systemd": len(cfg.Systemd),
		"systemdUser": len(cfg.SystemdUser), "bootstrap": len(cfg.Bootstrap),
		"agentInstall": len(cfg.AgentInstall), "firewall": len(cfg.Firewall), "commands": len(cfg.Commands),
	}
	for collection, count := range counts {
		if count != 1 {
			t.Errorf("resolved %s count = %d, want 1", collection, count)
		}
	}
}

func boolValue(value bool) *bool { return &value }
