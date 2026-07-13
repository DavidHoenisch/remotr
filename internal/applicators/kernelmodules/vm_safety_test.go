//go:build vmsafety

package kernelmodules_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/kernelmodules"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-KHB-005, OS-KHB-006: this isolated VM test performs real module loading
// and then proves the provider refuses a protected unload before mutation.
func TestKernelModuleSafetyVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("kernel-module VM test runs as root in the isolated Vagrant guest")
	}
	loaded, persistent := true, true
	dir := t.TempDir()
	provider := kernelmodules.New(models.KernelModuleResource{
		Name:       "loop",
		Module:     "loop",
		Loaded:     &loaded,
		Persistent: &persistent,
	}, nil)
	provider.ModulesLoadDir = filepath.Join(dir, "modules-load.d")
	provider.ModprobeDir = filepath.Join(dir, "modprobe.d")
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() real loop module = %v", err)
	}
	if check := provider.Check(context.Background()); check.Status != "compliant" {
		t.Fatalf("Check() after real Apply = %+v", check)
	}

	remove := false
	protected := kernelmodules.New(models.KernelModuleResource{
		Name:             "loop-protected",
		Module:           "loop",
		Loaded:           &remove,
		ProtectedModules: []string{"loop"},
	}, nil)
	if err := protected.Apply(context.Background()); err == nil {
		t.Fatal("protected module unload unexpectedly succeeded")
	}

	mountInfo := filepath.Join(dir, "mountinfo")
	if err := os.WriteFile(mountInfo, []byte("24 23 8:1 / / rw,relatime - loop /dev/vda1 rw\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rootProtected := kernelmodules.New(models.KernelModuleResource{Name: "loop-root", Module: "loop", Loaded: &remove}, nil)
	rootProtected.ProcMountInfo = mountInfo
	rootProtected.SysRoot = filepath.Join(dir, "sys")
	if err := rootProtected.Apply(context.Background()); err == nil {
		t.Fatal("root filesystem module unload unexpectedly succeeded")
	}
}
