package userfiles_test

import (
	"context"
	"fmt"
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

func TestApplicator_authoritativeSelectorRemovesOnlyPreviouslyOwnedDepartedUserFile(t *testing.T) {
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
	resource := models.UserFileResource{
		ResourceMeta: models.ResourceMeta{Ownership: models.OwnershipAuthoritative},
		Name:         "motd", Selector: &models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionExplicit, Usernames: []string{"alice", "bob"}},
		Path: ".remotr-motd", Content: "hello\n",
	}
	first := userfiles.New(resource)
	first.StateDir, first.StateKey = root, "workstation/motd"
	first.ListUsers = func() ([]interactiveuser.Account, error) { return users, nil }
	if err := first.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}

	resource.Selector.Usernames = []string{"alice"}
	second := userfiles.New(resource)
	second.StateDir, second.StateKey = root, "workstation/motd"
	second.ListUsers = func() ([]interactiveuser.Account, error) { return users, nil }
	if check := second.Check(context.Background()); check.Status != executor.Drifted {
		t.Fatalf("selector transition Check() = %+v", check)
	}
	if err := second.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(users[0].HomeDir, ".remotr-motd")); err != nil {
		t.Fatalf("selected user file was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(users[1].HomeDir, ".remotr-motd")); !os.IsNotExist(err) {
		t.Fatalf("departed user's owned file remains: %v", err)
	}
}

func TestApplicator_mergeSelectorPreservesPreviouslyOwnedDepartedUserFile(t *testing.T) {
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
	resource := models.UserFileResource{
		ResourceMeta: models.ResourceMeta{Ownership: models.OwnershipMerge},
		Name:         "motd", Selector: &models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionExplicit, Usernames: []string{"alice", "bob"}},
		Path: ".remotr-motd", Content: "hello\n",
	}
	first := userfiles.New(resource)
	first.StateDir, first.StateKey = root, "workstation/motd"
	first.ListUsers = func() ([]interactiveuser.Account, error) { return users, nil }
	if err := first.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	resource.Selector.Usernames = []string{"alice"}
	second := userfiles.New(resource)
	second.StateDir, second.StateKey = root, "workstation/motd"
	second.ListUsers = func() ([]interactiveuser.Account, error) { return users, nil }
	if check := second.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("merge selector transition Check() = %+v", check)
	}
	if _, err := os.Stat(filepath.Join(users[1].HomeDir, ".remotr-motd")); err != nil {
		t.Fatalf("merge cleanup removed departed user's file: %v", err)
	}
}

// OS-IUP-009: one divergent user keeps the aggregate non-compliant while all
// selected usernames remain inspectable through safe structured subresults.
func TestApplicator_aggregatesPerUserCheckResults(t *testing.T) {
	root := t.TempDir()
	uid, gid := os.Getuid(), os.Getgid()
	users := []interactiveuser.Account{
		{Username: "alice", UID: uid, GID: gid, HomeDir: filepath.Join(root, "alice")},
		{Username: "bob", UID: uid, GID: gid, HomeDir: filepath.Join(root, "bob")},
		{Username: "carol", UID: uid, GID: gid, HomeDir: filepath.Join(root, "carol")},
	}
	for _, user := range users {
		if err := os.MkdirAll(user.HomeDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, user := range users[:2] {
		if err := os.WriteFile(filepath.Join(user.HomeDir, ".remotr-motd"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := userfiles.New(models.UserFileResource{Name: "motd", Users: "interactive", Path: ".remotr-motd", Content: "hello\n"})
	a.ListUsers = func() ([]interactiveuser.Account, error) { return users, nil }

	check := a.Check(context.Background())
	if check.Status != executor.Drifted || len(check.Subresults) != 3 {
		t.Fatalf("Check() = %+v, want three per-user results and aggregate drift", check)
	}
	if check.Subresults[0].Target != "alice" || check.Subresults[0].Status != executor.Compliant ||
		check.Subresults[1].Target != "bob" || check.Subresults[1].Status != executor.Compliant ||
		check.Subresults[2].Target != "carol" || check.Subresults[2].Status != executor.Drifted {
		t.Fatalf("subresults = %+v", check.Subresults)
	}
}

func TestApplicator_boundsPerUserCheckResults(t *testing.T) {
	root := t.TempDir()
	users := make([]interactiveuser.Account, executor.MaxCheckSubresults+5)
	for i := range users {
		users[i] = interactiveuser.Account{Username: fmt.Sprintf("user-%02d", i), UID: os.Getuid(), GID: os.Getgid(), HomeDir: filepath.Join(root, fmt.Sprintf("user-%02d", i))}
		if err := os.MkdirAll(users[i].HomeDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a := userfiles.New(models.UserFileResource{Name: "motd", Users: "interactive", Path: ".remotr-motd", Content: "hello\n"})
	a.ListUsers = func() ([]interactiveuser.Account, error) { return users, nil }
	check := a.Check(context.Background())
	if len(check.Subresults) != executor.MaxCheckSubresults || !check.SubresultsTruncated {
		t.Fatalf("Check() subresults=%d truncated=%t", len(check.Subresults), check.SubresultsTruncated)
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

// OS-IUP-003: a failure for one selected malicious home is isolated from both
// the external target and the successfully managed user's home.
func TestInteractivePolicyIntegration_rejectsMaliciousHomeSymlinkAcrossUsers(t *testing.T) {
	root := t.TempDir()
	uid, gid := os.Getuid(), os.Getgid()
	users := []interactiveuser.Account{
		{Username: "alice", UID: uid, GID: gid, HomeDir: filepath.Join(root, "alice")},
		{Username: "mallory", UID: uid, GID: gid, HomeDir: filepath.Join(root, "mallory")},
	}
	for _, user := range users {
		if err := os.MkdirAll(user.HomeDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	external := filepath.Join(root, "external")
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(users[1].HomeDir, ".config")); err != nil {
		t.Fatal(err)
	}
	provider := userfiles.New(models.UserFileResource{
		Name: "app-policy", Selector: &models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionAll},
		Path: ".config/policy.conf", Content: "managed=true\n",
	})
	provider.ListUsers = func() ([]interactiveuser.Account, error) { return users, nil }
	if err := provider.Apply(context.Background()); err == nil {
		t.Fatal("Apply() accepted malicious home symlink")
	}
	if body, err := os.ReadFile(filepath.Join(users[0].HomeDir, ".config", "policy.conf")); err != nil || string(body) != "managed=true\n" {
		t.Fatalf("safe selected user result = %q, %v", body, err)
	}
	if _, err := os.Stat(filepath.Join(external, "policy.conf")); !os.IsNotExist(err) {
		t.Fatalf("malicious symlink modified external target: %v", err)
	}
	check := provider.Check(context.Background())
	if check.Status != executor.Drifted || len(check.Subresults) != 2 || check.Subresults[0].Status != executor.Compliant || check.Subresults[1].Target != "mallory" {
		t.Fatalf("aggregate Check() = %+v", check)
	}
}
