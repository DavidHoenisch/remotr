package links_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/links"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

// OS-FOM-007: a symbolic link checks its target and replaces a drifted target
// only when replacement is explicitly allowed.
func TestSymbolicLinkApplyReplacesDriftedTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "current")
	if err := os.Symlink("old-release", path); err != nil {
		t.Fatal(err)
	}
	provider := links.New(models.LinkResource{
		Name:                 "current-release",
		Path:                 path,
		Target:               "new-release",
		LinkType:             models.LinkTypeSymbolic,
		AllowTypeReplacement: true,
	})

	if _, met := provider.State(context.Background()); met {
		t.Fatal("different symlink target must drift")
	}
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if target, err := os.Readlink(path); err != nil || target != "new-release" {
		t.Fatalf("target = %q, %v; want new-release", target, err)
	}
	if _, met := provider.State(context.Background()); !met {
		t.Fatal("symlink must be compliant after replacement")
	}
}

// OS-FOM-007: a hard link is compliant only when it shares the requested
// source inode, not merely when a file exists at its destination.
func TestHardLinkApplyCreatesRequestedInode(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	path := filepath.Join(dir, "linked")
	if err := os.WriteFile(source, []byte("managed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := links.New(models.LinkResource{
		Name:     "linked-source",
		Path:     path,
		Target:   source,
		LinkType: models.LinkTypeHard,
	})

	if _, met := provider.State(context.Background()); met {
		t.Fatal("missing hard link must drift")
	}
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	sourceInfo, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	linkInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(sourceInfo, linkInfo) {
		t.Fatal("hard link must share the requested source inode")
	}
	if _, met := provider.State(context.Background()); !met {
		t.Fatal("hard link must be compliant after apply")
	}
}

// OS-AEC-097: validation at the ownership boundary must complete before a
// drifted active link is replaced, so failure preserves the previous target.
func TestSymbolicLinkInvalidOwnerPreservesActiveTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "current")
	if err := os.Symlink("active-release", path); err != nil {
		t.Fatal(err)
	}
	provider, err := contract.New(links.New(models.LinkResource{
		Name: "current", Path: path, Target: "new-release", LinkType: models.LinkTypeSymbolic,
		Owner: "remotr-owner-that-does-not-exist", AllowTypeReplacement: true,
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
	}))
	if err != nil {
		t.Fatal(err)
	}

	result := provider.Apply(context.Background())
	if result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("Apply = %+v, want failed owner resolution", result)
	}
	if target, err := os.Readlink(path); err != nil || target != "active-release" {
		t.Fatalf("active target after failed Apply = %q, %v", target, err)
	}
}

func TestHardLinkMissingSourcePreservesActiveDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "active")
	if err := os.WriteFile(path, []byte("active\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider, err := contract.New(links.New(models.LinkResource{
		Name: "active", Path: path, Target: filepath.Join(dir, "missing-source"), LinkType: models.LinkTypeHard,
		AllowTypeReplacement: true,
		ResourceMeta:         models.ResourceMeta{Lifecycle: models.LifecyclePresent},
	}))
	if err != nil {
		t.Fatal(err)
	}

	result := provider.Apply(context.Background())
	if result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("Apply = %+v, want failed missing source", result)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "active\n" {
		t.Fatalf("active destination after failed Apply = %q, %v", got, err)
	}
}
