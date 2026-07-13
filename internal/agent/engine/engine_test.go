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

func TestEngine_buildsNodeForEveryRegisteredResourceCollection(t *testing.T) {
	state := resolve.ResolvedState{Configurations: []models.Configuration{{
		Name:         "cfg",
		Packages:     []models.Package{{Name: "package", Present: true, PM: types.Apt}},
		Files:        []models.File{{Name: "file", Path: "/tmp/file"}},
		Directories:  []models.DirectoryResource{{Name: "directory", Path: "/tmp/directory", ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}}},
		Links:        []models.LinkResource{{Name: "link", Path: "/tmp/link", Target: "target", LinkType: models.LinkTypeSymbolic, ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}}},
		Groups:       []models.GroupResource{{Name: "group", Group: "example", ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}}},
		UserFiles:    []models.UserFileResource{{Name: "user-file", Users: "interactive", Path: ".config/file"}},
		Downloads:    []models.DownloadResource{{Name: "download", URL: "https://example.com/file", Dest: "/tmp/download"}},
		Users:        []models.UserResource{{Name: "user", Username: "example", Present: true}},
		Systemd:      []models.SystemdResource{{Name: "systemd", Unit: "example.service"}},
		SystemdUser:  []models.SystemdUserResource{{Name: "systemd-user", Unit: "example.service", Users: "interactive"}},
		Bootstrap:    []models.BootstrapResource{{Name: "bootstrap"}},
		AgentInstall: []models.AgentInstallResource{{Name: "agent-install"}},
		Firewall:     []models.FirewallResource{{Name: "firewall", Action: "allow"}},
		Commands:     []models.CommandResource{{Name: "command", Check: []string{"true"}}},
	}}}

	eng, err := engine.New(state, facts.Facts{Distro: types.Debian, Arch: types.X86}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, address := range eng.NodeOrder() {
		got[address] = true
	}
	for _, name := range []string{"package", "file", "directory", "link", "group", "user-file", "download", "user", "systemd", "systemd-user", "bootstrap", "agent-install", "firewall", "command"} {
		address := "cfg/" + name
		if !got[address] {
			t.Errorf("engine omitted registered resource %q; order = %v", address, eng.NodeOrder())
		}
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
