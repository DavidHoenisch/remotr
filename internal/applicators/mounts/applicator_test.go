package mounts_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/mounts"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

// OS-MSM-001, OS-MSM-002: runtime activation and the precisely owned fstab
// declaration converge independently without touching unrelated entries.
func TestApplicator_MountsAndWritesOnlyItsOwnedFstabEntry(t *testing.T) {
	mounted, persistent := true, true
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	target := filepath.Join(dir, "target")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	fstab := filepath.Join(dir, "fstab")
	if err := os.WriteFile(fstab, []byte("UUID=other /other ext4 defaults 0 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mountInfo := filepath.Join(dir, "mountinfo")
	if err := os.WriteFile(mountInfo, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"mount [-t tmpfs -o mode=0755 tmpfs " + target + "]": {},
	}}
	applicator := mounts.New(models.MountResource{Name: "cache", Source: "tmpfs", Target: target, FilesystemType: "tmpfs", Options: []string{"mode=0755"}, Mounted: &mounted, Persistent: &persistent}, runner)
	applicator.FstabPath = fstab
	applicator.MountInfoPath = mountInfo
	applicator.StateDir = filepath.Join(dir, "state")

	if err := applicator.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if len(runner.Calls) != 1 || runner.Calls[0].Name != "mount" || !slices.Equal(runner.Calls[0].Args, []string{"-t", "tmpfs", "-o", "mode=0755", "tmpfs", target}) {
		t.Fatalf("mount calls = %#v", runner.Calls)
	}
	contents, err := os.ReadFile(fstab)
	if err != nil {
		t.Fatal(err)
	}
	want := "UUID=other /other ext4 defaults 0 2\ntmpfs " + target + " tmpfs mode=0755 0 0 # remotr:cache\n"
	if string(contents) != want {
		t.Fatalf("fstab = %q, want %q", contents, want)
	}
}

// OS-MSM-005: normal busy-target failure is not silently escalated to lazy or
// forced unmount behavior.
func TestApplicator_NormalUnmountDoesNotEscalate(t *testing.T) {
	mounted := false
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	mountInfo := filepath.Join(dir, "mountinfo")
	if err := os.WriteFile(mountInfo, []byte("36 25 0:28 / "+target+" rw - tmpfs tmpfs rw\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"umount [" + target + "]": {Stderr: []byte("target is busy"), Err: errors.New("busy")},
	}}
	applicator := mounts.New(models.MountResource{Name: "cache", Source: "tmpfs", Target: target, FilesystemType: "tmpfs", Mounted: &mounted}, runner)
	applicator.MountInfoPath = mountInfo
	applicator.FstabPath = filepath.Join(dir, "fstab")
	applicator.StateDir = filepath.Join(dir, "state")

	if err := applicator.Apply(context.Background()); err == nil {
		t.Fatal("Apply() unexpectedly succeeded")
	}
	if len(runner.Calls) != 1 || runner.Calls[0].Name != "umount" || !slices.Equal(runner.Calls[0].Args, []string{target}) {
		t.Fatalf("runner calls = %#v", runner.Calls)
	}
}

// OS-MSM-002, OS-MSM-003: a persistent-only resource never activates a
// mount, and removal is keyed by its exact stable ownership marker.
func TestApplicator_PersistentOnlyRemovalPreservesOtherEntries(t *testing.T) {
	persistent := false
	dir := t.TempDir()
	fstab := filepath.Join(dir, "fstab")
	contents := "tmpfs /cache tmpfs defaults 0 0 # remotr:cache\n" +
		"tmpfs /cache2 tmpfs defaults 0 0 # remotr:cache2\n" +
		"UUID=other /other ext4 defaults 0 2\n"
	if err := os.WriteFile(fstab, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	applicator := mounts.New(models.MountResource{Name: "cache", Source: "tmpfs", Target: filepath.Join(dir, "cache"), FilesystemType: "tmpfs", Persistent: &persistent}, &executil.MockRunner{})
	applicator.FstabPath = fstab
	applicator.MountInfoPath = filepath.Join(dir, "missing-mountinfo")
	applicator.StateDir = filepath.Join(dir, "state")

	if err := applicator.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	got, err := os.ReadFile(fstab)
	if err != nil {
		t.Fatal(err)
	}
	want := "tmpfs /cache2 tmpfs defaults 0 0 # remotr:cache2\nUUID=other /other ext4 defaults 0 2\n"
	if string(got) != want {
		t.Fatalf("fstab = %q, want %q", got, want)
	}
}

// OS-MSM-001 / OS-AEC-098: persistent-only declarations still participate in
// boot, so a locally unsupported filesystem must fail preflight before the
// provider changes fstab.
func TestProviderRejectsUnsupportedPersistentFilesystemBeforeFstab(t *testing.T) {
	persistent := true
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	fstab := filepath.Join(dir, "fstab")
	original := []byte("UUID=other /other ext4 defaults 0 2\n")
	if err := os.WriteFile(fstab, original, 0o640); err != nil {
		t.Fatal(err)
	}
	filesystems := filepath.Join(dir, "filesystems")
	if err := os.WriteFile(filesystems, []byte("nodev\ttmpfs\n\text4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	applicator := mounts.New(models.MountResource{
		Name: "unsupported-boot", Source: "tmpfs", Target: target,
		FilesystemType: "remotr_missingfs", Persistent: &persistent,
	}, &executil.MockRunner{})
	applicator.FstabPath = fstab
	applicator.FilesystemsPath = filesystems
	applicator.StateDir = filepath.Join(dir, "state")
	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}

	result := provider.Apply(context.Background())
	if result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("unsupported persistent filesystem Apply = %+v, want failed", result)
	}
	got, err := os.ReadFile(fstab)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, original) {
		t.Fatalf("fstab after unsupported persistent filesystem = %q, want %q", got, original)
	}
}

// OS-MSM-004: a runtime mount that would obscure the live Remotr state path
// is rejected before either the runner or fstab can be touched.
func TestApplicator_BlocksStateDirectoryBeforeMutation(t *testing.T) {
	mounted := true
	dir := t.TempDir()
	target := filepath.Join(dir, "state")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	fstab := filepath.Join(dir, "fstab")
	if err := os.WriteFile(fstab, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{}
	applicator := mounts.New(models.MountResource{Name: "state", Source: "tmpfs", Target: target, FilesystemType: "tmpfs", Mounted: &mounted}, runner)
	applicator.FstabPath = fstab
	applicator.MountInfoPath = filepath.Join(dir, "mountinfo")
	if err := os.WriteFile(applicator.MountInfoPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	applicator.StateDir = target

	if err := applicator.Apply(context.Background()); err == nil {
		t.Fatal("Apply() unexpectedly allowed mount over state directory")
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("runner calls = %#v, want none", runner.Calls)
	}
	got, err := os.ReadFile(fstab)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("fstab changed to %q", got)
	}
}

// OS-MSM-004 / OS-AEC-098: the protected-path policy applies to local mount
// sources as well as targets. Exposing the live Remotr state tree through a
// second mount point is blocked before the native mount boundary.
func TestProviderBlocksProtectedStateSourceBeforeMutation(t *testing.T) {
	mounted := true
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	target := filepath.Join(dir, "target")
	for _, path := range []string{stateDir, target} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	filesystems := filepath.Join(dir, "filesystems")
	if err := os.WriteFile(filesystems, []byte("nodev\tnone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{}
	applicator := mounts.New(models.MountResource{
		Name: "protected-source", Source: stateDir, Target: target,
		FilesystemType: "none", Options: []string{"bind"}, Mounted: &mounted,
	}, runner)
	applicator.FstabPath = filepath.Join(dir, "fstab")
	applicator.MountInfoPath = filepath.Join(dir, "mountinfo")
	if err := os.WriteFile(applicator.MountInfoPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	applicator.FilesystemsPath = filesystems
	applicator.StateDir = stateDir
	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}

	result := provider.Apply(context.Background())
	if result.Status != contract.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "protected mount source") {
		t.Fatalf("protected source Apply = %+v, want pre-mutation control-path failure", result)
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("protected source native calls = %#v, want none", runner.Calls)
	}
}

// OS-MSM-003 / OS-AEC-098: when a resource manages persistence and runtime
// state together, failure to stage the boot declaration must happen before the
// provider changes the live mount table.
func TestProviderDoesNotMountWhenFstabPersistenceFails(t *testing.T) {
	mounted, persistent := true, true
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	mountInfo := filepath.Join(dir, "mountinfo")
	if err := os.WriteFile(mountInfo, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"mount [-t tmpfs tmpfs " + target + "]": {},
	}}
	applicator := mounts.New(models.MountResource{
		Name: "transactional", Source: "tmpfs", Target: target,
		FilesystemType: "tmpfs", Mounted: &mounted, Persistent: &persistent,
	}, runner)
	applicator.FstabPath = filepath.Join("/proc", "remotr-mount-test", "fstab")
	applicator.MountInfoPath = mountInfo
	applicator.StateDir = filepath.Join(dir, "state")
	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}

	result := provider.Apply(context.Background())
	if result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("persistence failure Apply = %+v, want failed", result)
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("native calls after rejected persistence = %#v, want none", runner.Calls)
	}
}
