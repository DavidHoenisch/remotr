//go:build vmsafety

package mounts_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/mounts"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-MSM-001 through OS-MSM-005 / OS-AEC-098: exercise an actual disposable
// tmpfs mount, its boot declaration, independent scope transitions, native
// fstab verification, preservation, and a no-change second Apply.
func TestMountProviderVM(t *testing.T) {
	if os.Geteuid() != 0 {
		// test-exception: EXC-019
		t.Skip("mount VM test runs as root in the isolated Vagrant guest")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	fstab := filepath.Join(dir, "fstab")
	unrelated := "# unrelated Ubuntu mount declaration remains byte-for-byte\n"
	if err := os.WriteFile(fstab, []byte(unrelated), 0o640); err != nil {
		t.Fatal(err)
	}
	mounted, persistent := true, true
	provider := mounts.New(models.MountResource{Name: "vm-cache", Source: "tmpfs", Target: target, FilesystemType: "tmpfs", Mounted: &mounted, Persistent: &persistent}, nil)
	provider.FstabPath = fstab
	provider.StateDir = filepath.Join(dir, "state")
	t.Cleanup(func() {
		remove := false
		cleanup := mounts.New(models.MountResource{Name: "vm-cache-cleanup", Source: "tmpfs", Target: target, FilesystemType: "tmpfs", Mounted: &remove}, nil)
		cleanup.StateDir = filepath.Join(dir, "state")
		if result := cleanup.ApplyResult(context.Background()); result.Status == executor.Failed {
			t.Errorf("cleanup unmount = %+v", result)
		}
	})
	if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("Check() after real Apply = %+v", check)
	}
	if result := provider.ApplyResult(context.Background()); result.Status != executor.NoChange {
		t.Fatalf("second ApplyResult() = %+v, want no-change", result)
	}
	wantFstab := unrelated + "tmpfs " + target + " tmpfs defaults 0 0 # remotr:vm-cache\n"
	contents, err := os.ReadFile(fstab)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != wantFstab {
		t.Fatalf("fstab after Apply = %q, want %q", contents, wantFstab)
	}
	info, err := os.Stat(fstab)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("fstab mode = %04o, want 0640", info.Mode().Perm())
	}
	if output, err := exec.Command("findmnt", "--verify", "--tab-file", fstab).CombinedOutput(); err != nil {
		t.Fatalf("findmnt rejected staged boot declaration: %v: %s", err, output)
	}

	persistent = false
	persistentOnly := mounts.New(models.MountResource{Name: "vm-cache", Source: "tmpfs", Target: target, FilesystemType: "tmpfs", Persistent: &persistent}, nil)
	persistentOnly.FstabPath = fstab
	persistentOnly.StateDir = filepath.Join(dir, "state")
	if result := persistentOnly.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("persistent-only removal = %+v, want changed", result)
	}
	runtimeOnly := mounts.New(models.MountResource{Name: "vm-cache", Source: "tmpfs", Target: target, FilesystemType: "tmpfs", Mounted: &mounted}, nil)
	runtimeOnly.StateDir = filepath.Join(dir, "state")
	if check := runtimeOnly.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("runtime state changed by persistent-only removal = %+v", check)
	}
	contents, err = os.ReadFile(fstab)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != unrelated {
		t.Fatalf("fstab after owned removal = %q, want %q", contents, unrelated)
	}

	mounted = false
	runtimeOnly = mounts.New(models.MountResource{Name: "vm-cache", Source: "tmpfs", Target: target, FilesystemType: "tmpfs", Mounted: &mounted}, nil)
	runtimeOnly.StateDir = filepath.Join(dir, "state")
	if result := runtimeOnly.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("runtime-only unmount = %+v, want changed", result)
	}
	if check := runtimeOnly.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("Check() after real unmount = %+v", check)
	}
}
