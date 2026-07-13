package files_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/files"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestApplicator_regexMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sshd_config")
	if err := os.WriteFile(path, []byte("PermitRootLogin no\n"), 0644); err != nil {
		t.Fatal(err)
	}
	a := files.New(models.File{
		Name:     "sshd",
		Path:     path,
		WithRegx: `(?m)^PermitRootLogin no`,
	})
	_, met := a.State(context.Background())
	if !met {
		t.Fatal("expected regex match")
	}
}

func TestApplicator_lineEditApply(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "login.defs")
	if err := os.WriteFile(path, []byte("# PASS_MAX_DAYS 999\nPASS_MIN_DAYS 0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	a := files.New(models.File{
		Name:           "pass-max-days",
		Path:           path,
		UpdateExisting: true,
		WithRegx:       `(?m)^PASS_MAX_DAYS[[:space:]]+90$`,
		ReplaceRegx:    `^#?\s*PASS_MAX_DAYS[[:space:]].*`,
		Content:        "PASS_MAX_DAYS 90",
	})
	_, met := a.State(context.Background())
	if met {
		t.Fatal("expected drift before apply")
	}
	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "PASS_MAX_DAYS 90\nPASS_MIN_DAYS 0\n" {
		t.Fatalf("content = %q", data)
	}
	_, met = a.State(context.Background())
	if !met {
		t.Fatal("expected compliant after apply")
	}
}

func TestApplicator_lineEditRevert(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "login.defs")
	original := "# PASS_MAX_DAYS 999\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	a := files.New(models.File{
		Name:           "pass-max-days",
		Path:           path,
		UpdateExisting: true,
		WithRegx:       `(?m)^PASS_MAX_DAYS[[:space:]]+90$`,
		ReplaceRegx:    `^#?\s*PASS_MAX_DAYS[[:space:]].*`,
		Content:        "PASS_MAX_DAYS 90",
	})
	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := a.Revert(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("revert = %q want %q", data, original)
	}
}

func TestApplicator_contentDrift(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "motd")
	if err := os.WriteFile(path, []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	a := files.New(models.File{Name: "motd", Path: path, Content: "new\n"})
	_, met := a.State(context.Background())
	if met {
		t.Fatal("expected content drift")
	}
	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new\n" {
		t.Fatalf("content = %q", data)
	}
}

func TestApplicator_metadataOnlyModeDrift(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "motd")
	if err := os.WriteFile(path, []byte("managed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := files.New(models.File{Name: "motd", Path: path, Content: "managed\n", Mode: []int{0o644}})
	if _, met := a.State(context.Background()); met {
		t.Fatal("expected mode-only drift")
	}
	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("mode = %o, want 644", got)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "managed\n" {
		t.Fatalf("content changed: %q, %v", data, err)
	}
	if _, met := a.State(context.Background()); !met {
		t.Fatal("expected compliance after metadata repair")
	}
}

func TestApplicator_absentRemovesManagedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "obsolete")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := files.New(models.File{ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: "obsolete", Path: path})
	if _, met := a.State(context.Background()); met {
		t.Fatal("expected present file to drift")
	}
	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("file remains: %v", err)
	}
	if _, met := a.State(context.Background()); !met {
		t.Fatal("expected absent file compliant")
	}
}

func TestApplicator_wholeFileReplacementIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	a := files.New(models.File{Name: "policy", Path: path, Content: "new\n", Mode: []int{0o640}})
	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Fatal("replacement reused the active inode; want staged rename")
	}
	if got := after.Mode().Perm(); got != 0o640 {
		t.Fatalf("mode = %o, want 640", got)
	}
	backup, err := os.ReadFile(path + ".remotr.bak")
	if err != nil || string(backup) != "old\n" {
		t.Fatalf("backup = %q, %v", backup, err)
	}
}

func TestApplicator_systemFileRejectsSymlinkRedirect(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "managed")
	if err := os.WriteFile(target, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	a := files.New(models.File{Name: "managed", Path: link, Content: "replace\n"})
	if _, met := a.State(context.Background()); met {
		t.Fatal("symlink must not be compliant")
	}
	if err := a.Apply(context.Background()); err == nil {
		t.Fatal("expected no-follow apply failure")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "keep\n" {
		t.Fatalf("target changed: %q, %v", data, err)
	}
}
