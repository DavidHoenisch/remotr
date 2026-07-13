package sysctl_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/sysctl"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-KHB-001: live and boot-time sysctl scopes are independently observed and
// converged through a single Remotr-owned drop-in.
func TestApplicator_convergesPersistentAndRuntimeSysctl(t *testing.T) {
	root := t.TempDir()
	procValue := filepath.Join(root, "proc", "sys", "net", "ipv4", "ip_forward")
	if err := os.MkdirAll(filepath.Dir(procValue), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(procValue, []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	applicator := sysctl.New(models.SysctlResource{
		Name: "forwarding", Key: "net.ipv4.ip_forward", Value: "1", Runtime: true, Persistent: true,
	}, nil)
	applicator.ProcRoot = filepath.Join(root, "proc", "sys")
	applicator.DropInDir = filepath.Join(root, "sysctl.d")

	if _, compliant := applicator.State(context.Background()); compliant {
		t.Fatal("State() unexpectedly reported drifted sysctl as compliant")
	}
	if err := applicator.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	runtime, err := os.ReadFile(procValue)
	if err != nil || string(runtime) != "1\n" {
		t.Fatalf("runtime value = %q, %v; want 1", runtime, err)
	}
	dropIn, err := os.ReadFile(filepath.Join(applicator.DropInDir, "99-remotr-forwarding.conf"))
	if err != nil || string(dropIn) != "net.ipv4.ip_forward = 1\n" {
		t.Fatalf("drop-in = %q, %v", dropIn, err)
	}
	if _, compliant := applicator.State(context.Background()); !compliant {
		t.Fatal("State() is not compliant after Apply")
	}
}

// OS-KHB-003: a key absent from the running kernel is unsupported rather than
// reported as ordinary drift.
func TestApplicator_reportsMissingRuntimeKeyAsUnsupported(t *testing.T) {
	applicator := sysctl.New(models.SysctlResource{
		Name: "missing", Key: "kernel.unavailable_key", Value: "1", Runtime: true,
	}, nil)
	applicator.ProcRoot = t.TempDir()
	if check := applicator.Check(context.Background()); check.Status != executor.Unsupported || check.ReasonCode != "sysctl_key_unsupported" {
		t.Fatalf("Check() = %+v, want unsupported key result", check)
	}
}

// OS-KHB-004: next-boot persistence writes only the owned drop-in and reports
// a deferred activation without touching a live kernel key.
func TestApplicator_nextBootReportsActivationWithoutRuntimeWrite(t *testing.T) {
	root := t.TempDir()
	applicator := sysctl.New(models.SysctlResource{
		Name: "deferred", Key: "vm.swappiness", Value: "10", Persistent: true, Activation: models.SysctlNextBoot,
	}, nil)
	applicator.ProcRoot = filepath.Join(root, "proc", "sys")
	applicator.DropInDir = filepath.Join(root, "sysctl.d")
	result := applicator.ApplyResult(context.Background())
	if result.Status != executor.Changed || !slices.Equal(result.Activation, []executor.ActivationSignal{{Kind: executor.ActivationNextBoot}}) {
		t.Fatalf("ApplyResult() = %+v, want changed next-boot activation", result)
	}
	if _, err := os.Stat(filepath.Join(root, "proc", "sys", "vm", "swappiness")); !os.IsNotExist(err) {
		t.Fatalf("next-boot activation changed runtime key: %v", err)
	}
}

func TestApplicator_reloadUsesOnlyItsOwnedDropIn(t *testing.T) {
	root := t.TempDir()
	dropInDir := filepath.Join(root, "sysctl.d")
	path := filepath.Join(dropInDir, "99-remotr-reload.conf")
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"sysctl [--load " + path + "]": {},
	}}
	applicator := sysctl.New(models.SysctlResource{
		Name: "reload", Key: "vm.swappiness", Value: "20", Persistent: true, Activation: models.SysctlReload,
	}, runner)
	applicator.DropInDir = dropInDir
	if err := applicator.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if len(runner.Calls) != 1 || runner.Calls[0].Name != "sysctl" || !slices.Equal(runner.Calls[0].Args, []string{"--load", path}) {
		t.Fatalf("reload calls = %#v, want exact owned-drop-in argv", runner.Calls)
	}
}
