package gitsync

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGitSyncer_syncFromRemoteWithBundledSeedFiles(t *testing.T) {
	remote := initBareRemote(t)

	seedDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(seedDir, "remotr.yaml"), []byte("seed: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(seedDir, "fleets", "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seedDir, "fleets", "default", "desired.yaml"), []byte("configurations: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := &memReleaseRef{}
	gs := &GitSyncer{
		RepoPath:  seedDir,
		RemoteURL: "file://" + remote,
		Branch:    "main",
		Store:     store,
	}

	if err := gs.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.ref == "" {
		t.Fatal("expected release ref")
	}
	if !isGitRepo(seedDir) {
		t.Fatal("expected git checkout after clearing bundled seed files")
	}
	if _, err := os.Stat(filepath.Join(seedDir, "desired.yaml")); err != nil {
		t.Fatalf("expected remote desired.yaml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(seedDir, "remotr.yaml")); !os.IsNotExist(err) {
		t.Fatal("expected bundled remotr.yaml to be replaced by remote checkout")
	}
}

func initBareRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	remote := filepath.Join(dir, "remote.git")
	runGit(t, dir, "init", "--bare", remote)

	work := filepath.Join(dir, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, work, "init")
	runGit(t, work, "config", "user.email", "test@example.com")
	runGit(t, work, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(work, "desired.yaml"), []byte("configurations: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, work, "add", "desired.yaml")
	runGit(t, work, "commit", "-m", "init")
	runGit(t, work, "branch", "-M", "main")
	runGit(t, work, "remote", "add", "origin", remote)
	runGit(t, work, "push", "-u", "origin", "main")
	return remote
}
