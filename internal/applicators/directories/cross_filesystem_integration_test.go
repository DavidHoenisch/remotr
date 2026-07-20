//go:build providerintegration

package directories_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/directories"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-AEC-097: the Ubuntu selector mounts mounted/ as a distinct tmpfs below
// root. Non-crossing purge preserves it, and recursive absence fails its full
// preflight without deleting a same-filesystem sibling.
func TestDirectoryCrossFilesystemBoundaryIsPreserved(t *testing.T) {
	root := "/qualification/root"
	mounted := filepath.Join(root, "mounted")
	if err := os.WriteFile(filepath.Join(mounted, "keep"), []byte("mounted"), 0o600); err != nil {
		t.Fatal(err)
	}
	removable := filepath.Join(root, "remove")
	if err := os.WriteFile(removable, []byte("remove"), 0o600); err != nil {
		t.Fatal(err)
	}

	purge := directories.New(models.DirectoryResource{
		Name: "root", Path: root, Recursive: true, Purge: true, CrossFilesystem: false,
		MaxDepth: 4, MaxEntries: 20,
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipAuthoritative},
	})
	if err := purge.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(removable); !os.IsNotExist(err) {
		t.Fatalf("same-filesystem purge target remains: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(mounted, "keep")); err != nil || string(got) != "mounted" {
		t.Fatalf("mounted child = %q, %v", got, err)
	}

	if err := os.WriteFile(removable, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	absent := directories.New(models.DirectoryResource{
		Name: "root", Path: root, Recursive: true, CrossFilesystem: false,
		MaxDepth: 4, MaxEntries: 20,
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent},
	})
	if err := absent.Apply(context.Background()); err == nil {
		t.Fatal("recursive absence crossed a protected filesystem boundary")
	}
	if got, err := os.ReadFile(removable); err != nil || string(got) != "preserve" {
		t.Fatalf("same-filesystem sibling changed before boundary rejection: %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(mounted, "keep")); err != nil || string(got) != "mounted" {
		t.Fatalf("mounted child after rejected absence = %q, %v", got, err)
	}
}
