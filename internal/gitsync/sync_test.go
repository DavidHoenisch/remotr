package gitsync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type memReleaseRef struct {
	ref string
}

func (m *memReleaseRef) GetReleaseRef(context.Context) (string, error) {
	return m.ref, nil
}

func (m *memReleaseRef) SetReleaseRef(_ context.Context, ref string) error {
	m.ref = ref
	return nil
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "desired.yaml"), []byte("configurations: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "desired.yaml")
	runGit(t, dir, "commit", "-m", "init")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestGitSyncer_advancesReleaseRefFromHEAD(t *testing.T) {
	repo := initGitRepo(t)
	store := &memReleaseRef{}

	gs := &GitSyncer{RepoPath: repo, Store: store}
	if err := gs.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.ref == "" {
		t.Fatal("expected release ref")
	}

	cmd := exec.Command("git", "-C", repo, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	want := string(out[:len(out)-1])
	if store.ref != want {
		t.Fatalf("ref = %q, want %q", store.ref, want)
	}
}

func TestGitSyncer_usesStaticRefWhenNotGitRepo(t *testing.T) {
	dir := t.TempDir()
	store := &memReleaseRef{}

	gs := &GitSyncer{RepoPath: dir, StaticRef: "compose-dev", Store: store}
	if err := gs.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.ref != "compose-dev" {
		t.Fatalf("ref = %q", store.ref)
	}
}

func TestGitSyncer_webhookRequiresSecret(t *testing.T) {
	repo := initGitRepo(t)
	gs := &GitSyncer{RepoPath: repo, WebhookSecret: "secret"}

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/git", nil)
	req.Header.Set(webhookHeader, "wrong")
	rec := httptest.NewRecorder()
	gs.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}
