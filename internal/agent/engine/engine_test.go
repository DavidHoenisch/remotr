package engine_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/resolve"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestEngine_cycleDetection(t *testing.T) {
	state := resolve.ResolvedState{Configurations: []models.Configuration{{
		Name: "cfg",
		Packages: []models.Package{
			{Name: "a", Present: true, ResourceMeta: models.ResourceMeta{DependsOn: []string{"cfg/b"}}},
			{Name: "b", Present: true, ResourceMeta: models.ResourceMeta{DependsOn: []string{"cfg/a"}}},
		},
	}}}
	_, err := engine.New(state, facts.Facts{Distro: types.Debian, Arch: types.X86}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestEngine_applyOrder(t *testing.T) {
	state := resolve.ResolvedState{Configurations: []models.Configuration{{
		Name: "cfg",
		AgentInstall: []models.AgentInstallResource{
			{Name: "agent", Version: "1", ArtifactURL: "https://x/a.tar.gz", ExtractDir: "d", FleetURL: "https://f", EnrollmentTokenSecret: "file:/etc/t", RunningCheck: models.AgentRunningCheck{Process: "agent"}},
		},
		Commands: []models.CommandResource{
			{Name: "cmd", Check: []string{"true"}},
		},
		Systemd: []models.SystemdResource{
			{Name: "svc", Unit: "sshd.service"},
		},
		Users: []models.UserResource{
			{Name: "u", Username: "dev", Present: true},
		},
		Files: []models.File{
			{Name: "f1", Path: "/tmp/motd", Content: "x"},
			{Name: "f2", Path: "/etc/ssh/sshd_config", Content: "y", ResourceMeta: models.ResourceMeta{PreApplyValidation: []string{"sshd -t"}}},
		},
		Downloads: []models.DownloadResource{
			{Name: "dl", URL: "https://example.com/bin", Dest: "/usr/local/bin/tool"},
		},
		Packages: []models.Package{
			{Name: "curl", Present: true},
		},
	}}}
	eng, err := engine.New(state, facts.Facts{Distro: types.Debian, Arch: types.X86}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	order := eng.NodeOrder()
	wantPrefix := []string{"cfg/curl", "cfg/f1", "cfg/dl", "cfg/f2", "cfg/u", "cfg/svc", "cfg/agent", "cfg/cmd"}
	if len(order) != len(wantPrefix) {
		t.Fatalf("order = %v", order)
	}
	for i, w := range wantPrefix {
		if order[i] != w {
			t.Fatalf("order[%d] = %q, want %q (full %v)", i, order[i], w, order)
		}
	}
}

// OS-SRM-008: coordinated reboot is a boot-tier operation and must be
// prepared before agent replacement or arbitrary destructive commands.
func TestEngineOrdersRebootInBootTier(t *testing.T) {
	state := resolve.ResolvedState{Configurations: []models.Configuration{{
		Name: "cfg",
		Reboots: []models.RebootResource{{
			Name: "maintenance", Generation: "kernel-6.12.1", Timeout: "15m",
		}},
		AgentInstall: []models.AgentInstallResource{{Name: "agent"}},
		Commands:     []models.CommandResource{{Name: "command", Check: []string{"true"}}},
	}}}
	eng, err := engine.New(state, facts.Facts{Distro: types.Debian, Arch: types.X86}, nil, nil, engine.WithStateDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"cfg/maintenance", "cfg/agent", "cfg/command"}
	if got := eng.NodeOrder(); !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestEngine_buildsNodeForEveryRegisteredResourceCollection(t *testing.T) {
	state := resolve.ResolvedState{Configurations: []models.Configuration{{
		Name:            "cfg",
		Packages:        []models.Package{{Name: "package", Present: true, PM: types.Apt}},
		APTRepositories: []models.APTRepository{{Name: "repository", URL: "https://packages.example.test/debian", Suites: []string{"stable"}, Components: []string{"main"}, SigningKey: "vendor"}},
		Sysctls:         []models.SysctlResource{{Name: "forwarding", Key: "net.ipv4.ip_forward", Value: "1", Runtime: true}},
		Files:           []models.File{{Name: "file", Path: "/tmp/file"}},
		Directories:     []models.DirectoryResource{{Name: "directory", Path: "/tmp/directory", ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}}},
		Links:           []models.LinkResource{{Name: "link", Path: "/tmp/link", Target: "target", LinkType: models.LinkTypeSymbolic, ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}}},
		Groups:          []models.GroupResource{{Name: "group", Group: "example", ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}}},
		UserFiles:       []models.UserFileResource{{Name: "user-file", Users: "interactive", Path: ".config/file", Content: "managed\n"}},
		DesktopSettings: []models.DesktopSettingResource{{Name: "desktop-setting", Provider: models.DesktopSettingProviderDconf, Scope: models.DesktopSettingScopeUser, Selector: models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionAll}, Path: "/org/gnome/desktop/interface/enable-animations", Value: models.DesktopSettingValue{Type: models.DesktopValueBoolean, Value: false}}},
		SessionPolicies: []models.SessionPolicyResource{{Name: "session-policy", Provider: models.DesktopSettingProviderGSettings, Selector: models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionAll}, LockEnabled: boolPointer(true)}},
		BrowserPolicies: []models.BrowserPolicyResource{{ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}, Name: "browser-policy", Browser: models.BrowserChromium, PolicyName: "HomepageLocation", Scope: models.BrowserPolicyScopeSystem, Level: models.BrowserPolicyLevelMandatory, Value: &models.BrowserPolicyValue{Type: models.BrowserValueString, Value: "https://example.test"}}},
		Downloads:       []models.DownloadResource{{Name: "download", URL: "https://example.com/file", Dest: "/tmp/download"}},
		Users:           []models.UserResource{{Name: "user", Username: "example", Present: true}},
		Systemd:         []models.SystemdResource{{Name: "systemd", Unit: "example.service"}},
		SystemdUser:     []models.SystemdUserResource{{Name: "systemd-user", Unit: "example.service", Users: "interactive"}},
		Reboots:         []models.RebootResource{{Name: "reboot", Generation: "g1", Timeout: "15m"}},
		Bootstrap:       []models.BootstrapResource{{Name: "bootstrap"}},
		AgentInstall:    []models.AgentInstallResource{{Name: "agent-install"}},
		Firewall:        []models.FirewallResource{{Name: "firewall", Action: "allow"}},
		Commands:        []models.CommandResource{{Name: "command", Check: []string{"true"}}},
	}}}

	eng, err := engine.New(state, facts.Facts{Distro: types.Debian, Arch: types.X86}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, address := range eng.NodeOrder() {
		got[address] = true
	}
	for _, name := range []string{"package", "repository", "forwarding", "file", "directory", "link", "group", "user-file", "desktop-setting", "session-policy", "browser-policy", "download", "user", "systemd", "systemd-user", "reboot", "bootstrap", "agent-install", "firewall", "command"} {
		address := "cfg/" + name
		if !got[address] {
			t.Errorf("engine omitted registered resource %q; order = %v", address, eng.NodeOrder())
		}
	}
}

func boolPointer(value bool) *bool { return &value }

// OS-IUP-007: trust references become dependency edges, so the verified
// trust-anchor resource always precedes browser policy activation.
func TestEngine_ordersTrustAnchorBeforeBrowserPolicyReference(t *testing.T) {
	state := resolve.ResolvedState{Configurations: []models.Configuration{{
		Name: "workstation",
		TrustAnchors: []models.TrustAnchorResource{{
			ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
			Name:         "corporate-root", AnchorRef: "remotr:trust-anchors/corporate-root@7",
			Fingerprint: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		}},
		BrowserPolicies: []models.BrowserPolicyResource{{
			ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
			Name:         "homepage", Browser: models.BrowserChromium, PolicyName: "HomepageLocation",
			Scope: models.BrowserPolicyScopeSystem, Level: models.BrowserPolicyLevelMandatory,
			Value:        &models.BrowserPolicyValue{Type: models.BrowserValueString, Value: "https://example.test"},
			TrustAnchors: []string{"workstation/corporate-root"},
		}},
	}}}
	eng, err := engine.New(state, facts.Facts{Distro: types.Debian, Browser: []facts.BrowserBackend{facts.BrowserChromium}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := eng.NodeOrder(), []string{"workstation/corporate-root", "workstation/homepage"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("NodeOrder() = %v, want %v", got, want)
	}
}

func TestEngine_firewallSurvivesResolveCheckAndReport(t *testing.T) {
	audit := true
	state := models.State{SchemaVersion: 1, Configurations: []models.Configuration{{Name: "cfg", Firewall: []models.FirewallResource{{ResourceMeta: models.ResourceMeta{Kind: models.ResourceKindFirewall}, Name: "allow-web", Audit: &audit, Action: "allow", Ports: []int{443}}}}}}
	resolved := resolve.Resolve(state, facts.Facts{Distro: types.Debian, Arch: types.X86})
	if len(resolved.Configurations) != 1 || len(resolved.Configurations[0].Firewall) != 1 {
		t.Fatalf("firewall dropped during resolve: %#v", resolved)
	}
	eng, err := engine.New(resolved, facts.Facts{Distro: types.Debian, Arch: types.X86}, &executil.MockRunner{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := eng.NodeOrder(); !reflect.DeepEqual(got, []string{"cfg/allow-web"}) {
		t.Fatalf("order = %v", got)
	}
	report := eng.CheckAll(context.Background())
	if len(report.Items) != 1 || report.Items[0].Address != "cfg/allow-web" || report.Items[0].Status != executor.Drifted || report.Items[0].ReasonCode != "audit_plan" {
		t.Fatalf("report = %+v", report)
	}
}

func TestEngine_reportsRuntimeProviderMismatchAsUnsupported(t *testing.T) {
	state := resolve.ResolvedState{Configurations: []models.Configuration{{
		Name:     "cfg",
		Packages: []models.Package{{Name: "curl", Present: true, PM: types.Apt}},
	}}}
	runner := &executil.MockRunner{}
	eng, err := engine.New(state, facts.Facts{Distro: types.Arch, Arch: types.X86, Package: types.Pacman}, runner, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := eng.ApplyAll(context.Background(), engine.PolicyAuto)
	if len(result.Applied) != 0 || len(result.Skipped) != 1 || result.Skipped[0] != "cfg/curl" || result.Failed != nil {
		t.Fatalf("ApplyAll() = %+v, want unsupported resource skipped", result)
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("provider mismatch invoked external commands: %+v", runner.Calls)
	}
}

func TestEngine_dependsOnOrder(t *testing.T) {
	state := resolve.ResolvedState{Configurations: []models.Configuration{{
		Name: "cfg",
		Packages: []models.Package{
			{Name: "base", Present: true},
			{Name: "app", Present: true, ResourceMeta: models.ResourceMeta{DependsOn: []string{"cfg/base"}}},
		},
	}}}
	eng, err := engine.New(state, facts.Facts{Distro: types.Debian, Arch: types.X86}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	order := eng.NodeOrder()
	if order[0] != "cfg/base" || order[1] != "cfg/app" {
		t.Fatalf("order = %v", order)
	}
}

func TestEngine_remotrPackageUsesPackageTier(t *testing.T) {
	state := resolve.ResolvedState{Configurations: []models.Configuration{{
		Name: "cfg",
		Files: []models.File{
			{Name: "f1", Path: "/tmp/motd", Content: "x"},
		},
		Packages: []models.Package{
			{Name: "curl", Present: true},
			{Name: "internal/mycli", Version: "1.0.0", Present: true, PM: types.Remotr},
		},
	}}}
	eng, err := engine.New(state, facts.Facts{Distro: types.Debian, Arch: types.X86}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	order := eng.NodeOrder()
	if order[0] != "cfg/curl" || order[1] != "cfg/internal/mycli" {
		t.Fatalf("remotr packages should apply in package tier; order = %v", order)
	}
	if order[2] != "cfg/f1" {
		t.Fatalf("order = %v", order)
	}
}

func TestEngine_reportPolicySkipsApply(t *testing.T) {
	state := resolve.ResolvedState{Configurations: []models.Configuration{{
		Name:     "cfg",
		Commands: []models.CommandResource{{Name: "always-drift", Check: []string{"false"}}},
	}}}
	eng, err := engine.New(state, facts.Facts{Distro: types.Debian, Arch: types.X86}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	drift := eng.CheckAll(context.Background())
	if drift.InCompliance {
		t.Fatal("expected drift")
	}
	result := eng.ApplyAll(context.Background(), engine.PolicyReport)
	if len(result.Applied) != 0 {
		t.Fatalf("expected no apply, got %v", result.Applied)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("expected skipped, got %v", result.Skipped)
	}
}
