package userfiles_test

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/userfiles"
	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func testAccounts(base string) ([]interactiveuser.Account, error) {
	uid := os.Getuid()
	gid := os.Getgid()
	home := filepath.Join(base, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		return nil, err
	}
	name := os.Getenv("USER")
	if name == "" {
		name = "testuser"
	}
	return []interactiveuser.Account{{
		Username: name,
		UID:      int(uid),
		GID:      int(gid),
		HomeDir:  home,
	}}, nil
}

func TestApplicator_contentModeOwnedByUser(t *testing.T) {
	dir := t.TempDir()
	users, err := testAccounts(dir)
	if err != nil {
		t.Fatal(err)
	}

	a := userfiles.New(models.UserFileResource{
		Name:    "motd",
		Users:   "interactive",
		Path:    ".remotr-motd",
		Content: "hello\n",
		Mode:    []int{0o644},
	})
	a.ListUsers = func() ([]interactiveuser.Account, error) { return users, nil }

	ctx := context.Background()
	if _, met := a.State(ctx); met {
		t.Fatal("expected drift")
	}
	if err := a.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		path := filepath.Join(u.HomeDir, ".remotr-motd")
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if stat, ok := st.Sys().(*syscall.Stat_t); !ok || int(stat.Uid) != u.UID {
			t.Fatalf("%s: ownership not uid %d", path, u.UID)
		}
	}
}

func TestApplicator_lineEdit(t *testing.T) {
	dir := t.TempDir()
	users, err := testAccounts(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		cfg := filepath.Join(u.HomeDir, ".config")
		if err := os.MkdirAll(cfg, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(cfg, "app.conf")
		if err := os.WriteFile(path, []byte("flag=off\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	a := userfiles.New(models.UserFileResource{
		Name:           "app-flag",
		Users:          "interactive",
		Path:           ".config/app.conf",
		UpdateExisting: true,
		WithRegx:       `(?m)^flag=on$`,
		ReplaceRegx:    `(?m)^flag=off$`,
		Content:        "flag=on",
	})
	a.ListUsers = func() ([]interactiveuser.Account, error) { return users, nil }

	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		path := filepath.Join(u.HomeDir, ".config", "app.conf")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "flag=on\n" {
			t.Fatalf("%s: %q", u.Username, data)
		}
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if stat, ok := st.Sys().(*syscall.Stat_t); !ok || int(stat.Uid) != u.UID {
			t.Fatalf("%s: ownership not uid %d", u.Username, u.UID)
		}
	}
}

func TestApplicator_rejectsSymlinkRedirect(t *testing.T) {
	dir := t.TempDir()
	users, err := testAccounts(dir)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "root-owned")
	original := []byte("do not change\n")
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(users[0].HomeDir, ".remotr-motd")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	a := userfiles.New(models.UserFileResource{
		Name:    "motd",
		Users:   "interactive",
		Path:    ".remotr-motd",
		Content: "owned by user\n",
	})
	a.ListUsers = func() ([]interactiveuser.Account, error) { return users, nil }

	if err := a.Apply(context.Background()); err == nil {
		t.Fatal("expected symlink redirect to be rejected")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("target changed: %q", data)
	}
}
