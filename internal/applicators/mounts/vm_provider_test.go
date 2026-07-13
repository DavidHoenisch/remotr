//go:build vmsafety

package mounts_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/mounts"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-MSM-001, OS-MSM-004: exercise an actual disposable tmpfs mount, then
// prove the provider reconverges and can safely unmount it again.
func TestMountProviderVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("mount VM test runs as root in the isolated Vagrant guest")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	fstab := filepath.Join(dir, "fstab")
	mounted, persistent := true, true
	provider := mounts.New(models.MountResource{Name: "vm-cache", Source: "tmpfs", Target: target, FilesystemType: "tmpfs", Mounted: &mounted, Persistent: &persistent}, nil)
	provider.FstabPath = fstab
	provider.StateDir = filepath.Join(dir, "state")
	if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	t.Cleanup(func() {
		remove := false
		cleanup := mounts.New(models.MountResource{Name: "vm-cache-cleanup", Source: "tmpfs", Target: target, FilesystemType: "tmpfs", Mounted: &remove}, nil)
		cleanup.StateDir = filepath.Join(dir, "state")
		if err := cleanup.Apply(context.Background()); err != nil {
			t.Errorf("cleanup unmount = %v", err)
		}
	})
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("Check() after real Apply = %+v", check)
	}
}
