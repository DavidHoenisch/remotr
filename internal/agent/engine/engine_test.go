package engine_test

import (
	"context"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/resolve"
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
	_, err := engine.New(state, facts.Facts{Distro: types.Debian, Arch: types.X86}, nil)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestEngine_applyOrder(t *testing.T) {
	state := resolve.ResolvedState{Configurations: []models.Configuration{{
		Name: "cfg",
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
		Packages: []models.Package{
			{Name: "curl", Present: true},
		},
	}}}
	eng, err := engine.New(state, facts.Facts{Distro: types.Debian, Arch: types.X86}, nil)
	if err != nil {
		t.Fatal(err)
	}
	order := eng.NodeOrder()
	wantPrefix := []string{"cfg/curl", "cfg/f1", "cfg/f2", "cfg/u", "cfg/svc", "cfg/cmd"}
	if len(order) != len(wantPrefix) {
		t.Fatalf("order = %v", order)
	}
	for i, w := range wantPrefix {
		if order[i] != w {
			t.Fatalf("order[%d] = %q, want %q (full %v)", i, order[i], w, order)
		}
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
	eng, err := engine.New(state, facts.Facts{Distro: types.Debian, Arch: types.X86}, nil)
	if err != nil {
		t.Fatal(err)
	}
	order := eng.NodeOrder()
	if order[0] != "cfg/base" || order[1] != "cfg/app" {
		t.Fatalf("order = %v", order)
	}
}

func TestEngine_reportPolicySkipsApply(t *testing.T) {
	state := resolve.ResolvedState{Configurations: []models.Configuration{{
		Name:     "cfg",
		Commands: []models.CommandResource{{Name: "always-drift", Check: []string{"false"}}},
	}}}
	eng, err := engine.New(state, facts.Facts{Distro: types.Debian, Arch: types.X86}, nil)
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
