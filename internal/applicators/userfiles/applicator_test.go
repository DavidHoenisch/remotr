package userfiles_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/userfiles"
	"github.com/DavidHoenisch/remotr/internal/executor"
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

// OS-IUP-001: an absent explicit target is reported and must never broaden
// the policy to every interactive account.
func TestApplicator_reportsUnresolvedExplicitUser(t *testing.T) {
	dir := t.TempDir()
	users, err := testAccounts(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := userfiles.New(models.UserFileResource{
		Name: "motd",
		Selector: &models.InteractiveUserSelector{
			Mode:      models.InteractiveUserSelectionExplicit,
			Usernames: []string{users[0].Username, "missing-user"},
		},
		Path:    ".remotr-motd",
		Content: "hello\n",
	})
	a.ListUsers = func() ([]interactiveuser.Account, error) { return users, nil }

	check := a.Check(context.Background())
	if check.Status != executor.CheckFailed || check.ReasonCode != executor.ReasonCode("unresolved_user_target") || !strings.Contains(string(check.ObservedSummary), "missing-user") {
		t.Fatalf("Check() = %+v, want unresolved explicit target", check)
	}
	if _, err := os.Stat(filepath.Join(users[0].HomeDir, ".remotr-motd")); !os.IsNotExist(err) {
		t.Fatalf("policy unexpectedly changed selected user's home: %v", err)
	}
}

func TestApplicator_appliesOnlyExplicitUsers(t *testing.T) {
	root := t.TempDir()
	uid, gid := os.Getuid(), os.Getgid()
	users := []interactiveuser.Account{
		{Username: "alice", UID: uid, GID: gid, HomeDir: filepath.Join(root, "alice")},
		{Username: "bob", UID: uid, GID: gid, HomeDir: filepath.Join(root, "bob")},
	}
	for _, user := range users {
		if err := os.MkdirAll(user.HomeDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a := userfiles.New(models.UserFileResource{
		Name: "motd", Selector: &models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionExplicit, Usernames: []string{"alice"}},
		Path: ".remotr-motd", Content: "hello\n",
	})
	a.ListUsers = func() ([]interactiveuser.Account, error) { return users, nil }
	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(users[0].HomeDir, ".remotr-motd")); err != nil {
		t.Fatalf("selected user file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(users[1].HomeDir, ".remotr-motd")); !os.IsNotExist(err) {
		t.Fatalf("unselected user was modified: %v", err)
	}
}

func TestApplicator_detectsMetadataOnlyModeDrift(t *testing.T) {
	dir := t.TempDir()
	users, err := testAccounts(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(users[0].HomeDir, ".remotr-motd")
	if err := os.WriteFile(path, []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := userfiles.New(models.UserFileResource{Name: "motd", Users: "interactive", Path: ".remotr-motd", Content: "hello\n", Mode: []int{0o644}})
	a.ListUsers = func() ([]interactiveuser.Account, error) { return users, nil }
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
