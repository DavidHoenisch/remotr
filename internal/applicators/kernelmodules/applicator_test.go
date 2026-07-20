package kernelmodules_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/kernelmodules"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

// OS-AEC-098: a failed live activation must restore the exact provider-owned
// boot and options fragments staged before modprobe crossed the OS boundary.
func TestApplicator_restoresOwnedFragmentsAfterFailedActivation(t *testing.T) {
	root := t.TempDir()
	procModules := filepath.Join(root, "proc", "modules")
	modulesLoadDir := filepath.Join(root, "modules-load.d")
	modprobeDir := filepath.Join(root, "modprobe.d")
	loadPath := filepath.Join(modulesLoadDir, "99-remotr-loop.conf")
	optionsPath := filepath.Join(modprobeDir, "99-remotr-loop.conf")
	for path, content := range map[string][]byte{
		procModules: []byte{},
		loadPath:    []byte("previous-module\n"),
		optionsPath: []byte("options loop max_loop=8\n"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o640); err != nil {
			t.Fatal(err)
		}
	}
	wantLoad, _ := os.ReadFile(loadPath)
	wantOptions, _ := os.ReadFile(optionsPath)
	activationErr := errors.New("synthetic modprobe failure")
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"modprobe [loop max_loop=64]": {Stderr: []byte("activation rejected"), Err: activationErr},
	}}
	loaded, persistent := true, true
	applicator := kernelmodules.New(models.KernelModuleResource{
		Name: "loop", Module: "loop", Loaded: &loaded, Persistent: &persistent,
		Parameters: map[string]string{"max_loop": "64"},
	}, runner)
	applicator.ProcModules = procModules
	applicator.ModulesLoadDir = modulesLoadDir
	applicator.ModprobeDir = modprobeDir
	applicator.HasModprobe = func() bool { return true }
	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}
	result := provider.Apply(context.Background())
	if result.Status != contract.Failed || !errors.Is(result.Err, activationErr) {
		t.Fatalf("failed activation Apply = %+v", result)
	}
	for path, want := range map[string][]byte{loadPath: wantLoad, optionsPath: wantOptions} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("restored fragment %s = %q, err=%v, want %q", path, got, err, want)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o640 {
			t.Fatalf("restored fragment %s mode = %v, err=%v, want 0640", path, info.Mode().Perm(), err)
		}
	}
}

// OS-AEC-098: persistence-only module state does not mutate the running
// kernel, so the public contract reports when that boot declaration activates.
func TestApplicator_persistenceOnlyReportsNextBoot(t *testing.T) {
	root := t.TempDir()
	procModules := filepath.Join(root, "proc", "modules")
	if err := os.MkdirAll(filepath.Dir(procModules), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(procModules, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	persistent := true
	runner := &executil.MockRunner{}
	applicator := kernelmodules.New(models.KernelModuleResource{
		Name: "loop", Module: "loop", Persistent: &persistent,
	}, runner)
	applicator.ProcModules = procModules
	applicator.ModulesLoadDir = filepath.Join(root, "modules-load.d")
	applicator.ModprobeDir = filepath.Join(root, "modprobe.d")
	applicator.HasModprobe = func() bool { return true }
	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}
	result := provider.Apply(context.Background())
	if result.Status != contract.Changed || result.Err != nil ||
		len(result.Activation) != 1 || result.Activation[0].Kind != contract.ActivationNextBoot {
		t.Fatalf("persistence-only Apply = %+v, want changed with next-boot activation", result)
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("persistence-only modprobe calls = %+v, want none", runner.Calls)
	}
	if check := provider.Check(context.Background()); check.Status != contract.Compliant {
		t.Fatalf("persistence-only second Check = %+v, want compliant", check)
	}
}

// OS-KHB-005: a kernel module's current loaded state, boot-time declaration,
// and declared parameters converge through named Remotr-owned fragments.
func TestApplicator_loadsAndPersistsModuleWithParameters(t *testing.T) {
	root := t.TempDir()
	modules := filepath.Join(root, "proc", "modules")
	if err := os.MkdirAll(filepath.Dir(modules), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modules, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, persistent := true, true
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"modprobe [loop max_loop=64]": {},
	}}
	applicator := kernelmodules.New(models.KernelModuleResource{
		Name:       "loop",
		Module:     "loop",
		Loaded:     &loaded,
		Persistent: &persistent,
		Parameters: map[string]string{"max_loop": "64"},
	}, runner)
	applicator.ProcModules = modules
	applicator.ModulesLoadDir = filepath.Join(root, "modules-load.d")
	applicator.ModprobeDir = filepath.Join(root, "modprobe.d")
	applicator.HasModprobe = func() bool { return true }

	if check := applicator.Check(context.Background()); check.Status != executor.Drifted {
		t.Fatalf("Check() = %+v, want drifted", check)
	}
	if err := applicator.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if len(runner.Calls) != 1 || runner.Calls[0].Name != "modprobe" || !slices.Equal(runner.Calls[0].Args, []string{"loop", "max_loop=64"}) {
		t.Fatalf("modprobe calls = %#v, want exact module argv", runner.Calls)
	}

	load, err := os.ReadFile(filepath.Join(applicator.ModulesLoadDir, "99-remotr-loop.conf"))
	if err != nil || string(load) != "loop\n" {
		t.Fatalf("module load fragment = %q, %v", load, err)
	}
	options, err := os.ReadFile(filepath.Join(applicator.ModprobeDir, "99-remotr-loop.conf"))
	if err != nil || string(options) != "options loop max_loop=64\n" {
		t.Fatalf("module options fragment = %q, %v", options, err)
	}
}

// OS-KHB-006: an unload request for a module explicitly protecting a control
// subsystem is rejected before either modprobe or an owned boot fragment runs.
func TestApplicator_blocksProtectedModuleUnloadBeforeMutation(t *testing.T) {
	root := t.TempDir()
	modules := filepath.Join(root, "proc", "modules")
	if err := os.MkdirAll(filepath.Dir(modules), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modules, []byte("e1000 1 0 - Live 0x0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded := false
	runner := &executil.MockRunner{}
	applicator := kernelmodules.New(models.KernelModuleResource{
		Name:             "network-driver",
		Module:           "e1000",
		Loaded:           &loaded,
		ProtectedModules: []string{"e1000"},
	}, runner)
	applicator.ProcModules = modules
	applicator.SysRoot = filepath.Join(root, "sys")
	applicator.HasModprobe = func() bool { return true }

	if err := applicator.Apply(context.Background()); err == nil {
		t.Fatal("Apply() unexpectedly unloaded a protected module")
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("modprobe calls = %#v, want no mutation before preflight", runner.Calls)
	}
}

// OS-KHB-005: parameter declarations persist boot-time modprobe state, but
// runtime parameter observation is unmanaged unless loaded: true is requested.
func TestApplicator_leavesLiveParametersUnmanagedWithoutLoadedScope(t *testing.T) {
	root := t.TempDir()
	modules := filepath.Join(root, "proc", "modules")
	parameter := filepath.Join(root, "sys", "module", "loop", "parameters", "max_loop")
	options := filepath.Join(root, "modprobe.d", "99-remotr-loop.conf")
	for path, content := range map[string][]byte{
		modules:   []byte("loop 1 0 - Live 0x0\n"),
		parameter: []byte("8\n"),
		options:   []byte("options loop max_loop=64\n"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	applicator := kernelmodules.New(models.KernelModuleResource{
		Name: "loop", Module: "loop", Parameters: map[string]string{"max_loop": "64"},
	}, nil)
	applicator.ProcModules = modules
	applicator.SysModuleRoot = filepath.Join(root, "sys", "module")
	applicator.ModprobeDir = filepath.Join(root, "modprobe.d")
	applicator.HasModprobe = func() bool { return true }

	if check := applicator.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("Check() = %+v, want boot-time parameter compliance without live scope", check)
	}
}

// OS-KHB-006: the root filesystem's module is a boot-critical dependency even
// when its block-device driver does not identify the filesystem module.
func TestApplicator_blocksRootFilesystemModuleUnloadBeforeMutation(t *testing.T) {
	root := t.TempDir()
	modules := filepath.Join(root, "proc", "modules")
	mountInfo := filepath.Join(root, "proc", "self", "mountinfo")
	for path, content := range map[string][]byte{
		modules:   []byte("ext4 1 0 - Live 0x0\n"),
		mountInfo: []byte("24 23 8:1 / / rw,relatime - ext4 /dev/vda1 rw\n"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	loaded := false
	runner := &executil.MockRunner{}
	applicator := kernelmodules.New(models.KernelModuleResource{Name: "root-fs", Module: "ext4", Loaded: &loaded}, runner)
	applicator.ProcModules = modules
	applicator.ProcMountInfo = mountInfo
	applicator.SysRoot = filepath.Join(root, "sys")
	applicator.HasModprobe = func() bool { return true }

	if err := applicator.Apply(context.Background()); err == nil {
		t.Fatal("Apply() unexpectedly unloaded the active root filesystem module")
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("modprobe calls = %#v, want no mutation before root preflight", runner.Calls)
	}
}
