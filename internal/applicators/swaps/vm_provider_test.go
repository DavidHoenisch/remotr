//go:build vmsafety

package swaps_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/swaps"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-MSM-006 / OS-AEC-098: exercise real Ubuntu swap-file and loop-device
// state, exact priority, independent persistence, boot parsing, convergence,
// and safe cleanup through the public provider seam.
func TestSwapProviderVM(t *testing.T) {
	if os.Geteuid() != 0 {
		// test-exception: EXC-019
		t.Skip("swap VM test runs as root in the isolated Vagrant guest")
	}
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "swapfile")
	fstab := filepath.Join(dir, "fstab")
	unrelated := "# preserve unrelated Ubuntu boot configuration\n"
	if err := os.WriteFile(fstab, []byte(unrelated), 0o640); err != nil {
		t.Fatal(err)
	}
	active, persistent := true, true
	resource := models.SwapResource{
		Name: "vm-file", Path: path, Type: "file", SizeBytes: 64 << 20,
		Priority: 7, Active: &active, Persistent: &persistent,
	}
	provider := swaps.New(resource, nil)
	provider.FstabPath = fstab
	t.Cleanup(func() {
		remove := false
		cleanupResource := resource
		cleanupResource.Active = &remove
		cleanupResource.Persistent = nil
		cleanupResource.AllowRemove = true
		if result := swaps.New(cleanupResource, nil).ApplyResult(ctx); result.Status == executor.Failed {
			t.Errorf("swap-file cleanup = %+v", result)
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Errorf("remove swap-file fixture: %v", err)
		}
	})

	if result := provider.ApplyResult(ctx); result.Status != executor.Changed {
		t.Fatalf("swap-file ApplyResult() = %+v, want changed", result)
	}
	if check := provider.Check(ctx); check.Status != executor.Compliant {
		t.Fatalf("swap-file Check() = %+v, want compliant", check)
	}
	if result := provider.ApplyResult(ctx); result.Status != executor.NoChange {
		t.Fatalf("swap-file second ApplyResult() = %+v, want no-change", result)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != resource.SizeBytes || info.Mode().Perm() != 0o600 {
		t.Fatalf("swap-file identity = size %d mode %04o", info.Size(), info.Mode().Perm())
	}
	wantFstab := unrelated + path + " none swap pri=7 0 0 # remotr:vm-file\n"
	contents, err := os.ReadFile(fstab)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != wantFstab {
		t.Fatalf("swap fstab = %q, want %q", contents, wantFstab)
	}
	info, err = os.Stat(fstab)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("swap fstab mode = %04o, want 0640", info.Mode().Perm())
	}
	if output, err := exec.Command("findmnt", "--verify", "--tab-file", fstab).CombinedOutput(); err != nil {
		t.Fatalf("findmnt rejected swap boot declaration: %v: %s", err, output)
	}

	persistent = false
	persistentOnly := resource
	persistentOnly.Active = nil
	persistentOnly.Persistent = &persistent
	persistentProvider := swaps.New(persistentOnly, nil)
	persistentProvider.FstabPath = fstab
	if result := persistentProvider.ApplyResult(ctx); result.Status != executor.Changed {
		t.Fatalf("persistent-only removal = %+v, want changed", result)
	}
	runtimeOnly := resource
	runtimeOnly.Persistent = nil
	if check := swaps.New(runtimeOnly, nil).Check(ctx); check.Status != executor.Compliant {
		t.Fatalf("runtime state changed by persistent-only removal = %+v", check)
	}
	contents, err = os.ReadFile(fstab)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != unrelated {
		t.Fatalf("fstab after owned swap removal = %q, want %q", contents, unrelated)
	}

	active = false
	runtimeOnly.Active = &active
	runtimeOnly.AllowRemove = true
	runtimeProvider := swaps.New(runtimeOnly, nil)
	if result := runtimeProvider.ApplyResult(ctx); result.Status != executor.Changed {
		t.Fatalf("runtime-only swapoff = %+v, want changed", result)
	}
	if check := runtimeProvider.Check(ctx); check.Status != executor.Compliant {
		t.Fatalf("Check() after real swapoff = %+v", check)
	}

	testSwapBlockDeviceVM(t, ctx, dir)
}

func testSwapBlockDeviceVM(t *testing.T, ctx context.Context, dir string) {
	t.Helper()
	backing := filepath.Join(dir, "swap-device.img")
	if output, err := exec.Command("fallocate", "--length", "64M", backing).CombinedOutput(); err != nil {
		t.Fatalf("create loop backing: %v: %s", err, output)
	}
	output, err := exec.Command("losetup", "--find", "--show", backing).CombinedOutput()
	if err != nil {
		t.Fatalf("attach loop device: %v: %s", err, output)
	}
	device := strings.TrimSpace(string(output))
	detached := false
	t.Cleanup(func() {
		remove := false
		cleanup := swaps.New(models.SwapResource{
			Name: "vm-device", Path: device, Type: "device", Active: &remove, AllowRemove: true,
		}, nil)
		if result := cleanup.ApplyResult(ctx); result.Status == executor.Failed {
			t.Errorf("swap-device cleanup = %+v", result)
		}
		if !detached {
			if output, err := exec.Command("losetup", "--detach", device).CombinedOutput(); err != nil {
				t.Errorf("detach loop device: %v: %s", err, output)
			}
		}
	})
	if output, err := exec.Command("mkswap", device).CombinedOutput(); err != nil {
		t.Fatalf("format loop swap device: %v: %s", err, output)
	}
	active := true
	provider := swaps.New(models.SwapResource{
		Name: "vm-device", Path: device, Type: "device", Priority: 9, Active: &active,
	}, nil)
	if result := provider.ApplyResult(ctx); result.Status != executor.Changed {
		t.Fatalf("swap-device ApplyResult() = %+v, want changed", result)
	}
	if check := provider.Check(ctx); check.Status != executor.Compliant {
		t.Fatalf("swap-device Check() = %+v, want compliant", check)
	}
	if result := provider.ApplyResult(ctx); result.Status != executor.NoChange {
		t.Fatalf("swap-device second ApplyResult() = %+v, want no-change", result)
	}
	active = false
	provider = swaps.New(models.SwapResource{
		Name: "vm-device", Path: device, Type: "device", Active: &active, AllowRemove: true,
	}, nil)
	if result := provider.ApplyResult(ctx); result.Status != executor.Changed {
		t.Fatalf("swap-device removal = %+v, want changed", result)
	}
	if output, err := exec.Command("losetup", "--detach", device).CombinedOutput(); err != nil {
		t.Fatalf("detach verified loop device: %v: %s", err, output)
	}
	detached = true
}
