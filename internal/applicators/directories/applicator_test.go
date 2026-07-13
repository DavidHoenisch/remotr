package directories_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/directories"
	"github.com/DavidHoenisch/remotr/internal/models"
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
