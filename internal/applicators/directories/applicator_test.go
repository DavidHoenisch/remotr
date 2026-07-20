package directories_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/directories"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

// OS-FOM-003: a directory provider observes and repairs metadata drift
// without replacing the directory itself.
func TestDirectoryApplyRepairsModeDrift(t *testing.T) {
	path := filepath.Join(t.TempDir(), "managed")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	provider := directories.New(models.DirectoryResource{
		Name: "managed",
		Path: path,
		Mode: []int{0o750},
	})

	if _, met := provider.State(context.Background()); met {
		t.Fatal("directory with a different mode must drift")
	}
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o750 {
		t.Fatalf("mode = %o, want 750", got)
	}
	if _, met := provider.State(context.Background()); !met {
		t.Fatal("directory must be compliant after metadata repair")
	}
}

// OS-FOM-008: no-follow traversal prevents a user-controlled parent symlink
// from redirecting a directory resource outside its declared path.
func TestDirectoryApplyRejectsSymlinkedParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	redirect := filepath.Join(root, "redirect")
	if err := os.Symlink(outside, redirect); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(redirect, "managed")
	provider := directories.New(models.DirectoryResource{Name: "managed", Path: path})

	if err := provider.Apply(context.Background()); err == nil {
		t.Fatal("expected symlinked parent to be rejected")
	}
	if _, err := os.Stat(filepath.Join(outside, "managed")); !os.IsNotExist(err) {
		t.Fatalf("operation escaped through symlink: %v", err)
	}
}

// OS-FOM-001: replacing a wrong object kind requires an explicit policy.
func TestDirectoryApplyRequiresTypeReplacementPermission(t *testing.T) {
	path := filepath.Join(t.TempDir(), "managed")
	if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := directories.New(models.DirectoryResource{Name: "managed", Path: path})
	if err := provider.Apply(context.Background()); err == nil {
		t.Fatal("expected wrong object kind to require replacement permission")
	}

	provider.Directory.AllowTypeReplacement = true
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("managed path = %v, %v; want directory", info, err)
	}
}

// OS-FOM-002: an absent directory converges by removing the named directory
// and reports compliance on the next check.
func TestDirectoryApplyRemovesAbsentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "obsolete")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := directories.New(models.DirectoryResource{
		Name: "obsolete",
		Path: path,
		ResourceMeta: models.ResourceMeta{
			Lifecycle: models.LifecycleAbsent,
		},
	})
	if _, met := provider.State(context.Background()); met {
		t.Fatal("existing directory must drift when absent is requested")
	}
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("directory remains: %v", err)
	}
	if _, met := provider.State(context.Background()); !met {
		t.Fatal("absent directory must be compliant after apply")
	}
}

// OS-FOM-009: an authoritative recursive directory may purge its owned tree,
// but must leave explicit exclusions untouched.
func TestDirectoryApplyPurgesOnlyOwnedNonExcludedChildren(t *testing.T) {
	root := filepath.Join(t.TempDir(), "managed")
	if err := os.MkdirAll(filepath.Join(root, "remove", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "remove", "nested", "file"), []byte("remove"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "keep"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := directories.New(models.DirectoryResource{
		Name:       "managed",
		Path:       root,
		Recursive:  true,
		Purge:      true,
		Exclusions: []string{"keep"},
		MaxDepth:   4,
		MaxEntries: 10,
		ResourceMeta: models.ResourceMeta{
			Lifecycle: models.LifecyclePresent,
			Ownership: models.OwnershipAuthoritative,
		},
	})

	if _, met := provider.State(context.Background()); met {
		t.Fatal("unmanaged child must cause purge drift")
	}
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "remove")); !os.IsNotExist(err) {
		t.Fatalf("owned child remains: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "keep")); err != nil || string(got) != "keep" {
		t.Fatalf("excluded child = %q, %v", got, err)
	}
	if _, met := provider.State(context.Background()); !met {
		t.Fatal("purged directory must be compliant")
	}
}

// OS-FOM-009: entry bounds are validated before removal, avoiding a partial
// authoritative purge when the declared safety budget is exhausted.
func TestDirectoryApplyRefusesPurgeBeyondEntryBound(t *testing.T) {
	root := filepath.Join(t.TempDir(), "managed")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	provider := directories.New(models.DirectoryResource{
		Name:       "managed",
		Path:       root,
		Recursive:  true,
		Purge:      true,
		MaxDepth:   1,
		MaxEntries: 1,
		ResourceMeta: models.ResourceMeta{
			Lifecycle: models.LifecyclePresent,
			Ownership: models.OwnershipAuthoritative,
		},
	})

	if err := provider.Apply(context.Background()); err == nil {
		t.Fatal("expected entry-bound failure")
	}
	for _, name := range []string{"one", "two"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("%s was removed despite rejected plan: %v", name, err)
		}
	}
}

// OS-AEC-097: a rejected authoritative cleanup plan must fail before any
// independently managed directory metadata is changed.
func TestDirectoryBoundFailurePreservesMetadataAndChildren(t *testing.T) {
	root := filepath.Join(t.TempDir(), "managed")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	provider, err := contract.New(directories.New(models.DirectoryResource{
		Name: "managed", Path: root, Mode: []int{0o750}, Recursive: true, Purge: true,
		MaxDepth: 1, MaxEntries: 1,
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipAuthoritative},
	}))
	if err != nil {
		t.Fatal(err)
	}

	result := provider.Apply(context.Background())
	if result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("Apply = %+v, want failed bounded cleanup", result)
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode after rejected cleanup = %o, want preserved 700", info.Mode().Perm())
	}
	for _, name := range []string{"one", "two"} {
		if got, err := os.ReadFile(filepath.Join(root, name)); err != nil || string(got) != name {
			t.Fatalf("child %s after rejected cleanup = %q, %v", name, got, err)
		}
	}
}

// OS-FOM-002, OS-AEC-097: an absent directory may remove a non-empty tree
// only when the author explicitly supplies bounded recursive policy.
func TestDirectoryRecursiveAbsenceConvergesWithinBounds(t *testing.T) {
	root := filepath.Join(t.TempDir(), "obsolete")
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "file"), []byte("obsolete"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider, err := contract.New(directories.New(models.DirectoryResource{
		Name: "obsolete", Path: root, Recursive: true, MaxDepth: 4, MaxEntries: 10,
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent},
	}))
	if err != nil {
		t.Fatal(err)
	}

	if check := provider.Check(context.Background()); check.Status != contract.Drifted {
		t.Fatalf("initial Check = %+v, want drifted", check)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("Apply = %+v, want changed", result)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("recursive absent directory remains: %v", err)
	}
	if check := provider.Check(context.Background()); check.Status != contract.Compliant {
		t.Fatalf("second Check = %+v, want compliant", check)
	}
}
