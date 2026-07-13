package users_test

import (
	"context"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/users"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestApplicator_UIDDriftRequiresExplicitReassignment(t *testing.T) {
	a := users.New(models.UserResource{Name: "alice", Username: "alice", Present: true, UID: 2000})
	a.LookupFunc = func(string) (*user.User, error) { return &user.User{Username: "alice", Uid: "1000"}, nil }
	if _, met := a.State(context.Background()); met {
		t.Fatal("expected uid drift")
	}
	called := false
	a.ModifyUIDFunc = func(string, int) error { called = true; return nil }
	if err := a.Apply(context.Background()); err == nil {
		t.Fatal("expected reassignment policy error")
	}
	if called {
		t.Fatal("usermod called without explicit reassignment policy")
	}
}

func TestApplicator_UIDDriftReassignsExactUID(t *testing.T) {
	a := users.New(models.UserResource{Name: "alice", Username: "alice", Present: true, UID: 2000, AllowUIDReassignment: true})
	a.LookupFunc = func(string) (*user.User, error) { return &user.User{Username: "alice", Uid: "1000"}, nil }
	a.ModifyUIDFunc = func(name string, uid int) error {
		if name != "alice" || uid != 2000 {
			t.Fatalf("modify = %q %d", name, uid)
		}
		return nil
	}
	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// OS-LIA-004: merge membership preserves unrelated groups while applying the
// requested primary and supplementary memberships in one argv-safe command.
func TestApplicator_MergeGroupMemberships(t *testing.T) {
	a := users.New(models.UserResource{
		Name:                    "alice",
		Username:                "alice",
		Present:                 true,
		PrimaryGroup:            "operators",
		SupplementaryGroups:     []string{"docker"},
		SupplementaryGroupsMode: models.GroupMembershipMerge,
	})
	a.LookupFunc = func(string) (*user.User, error) {
		return &user.User{Username: "alice", Uid: "1000", Gid: "1000"}, nil
	}
	a.LookupGroupFunc = func(name string) (*user.Group, error) {
		return map[string]*user.Group{
			"operators": {Name: "operators", Gid: "2000"},
			"docker":    {Name: "docker", Gid: "3000"},
		}[name], nil
	}
	a.GroupIDsFunc = func(*user.User) ([]string, error) { return []string{"1000", "2000"}, nil }
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"usermod [--gid operators --append --groups docker -- alice]": {},
	}}
	a.Runner = runner

	if _, met := a.State(context.Background()); met {
		t.Fatal("primary and supplementary group drift must be observed")
	}
	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []executil.MockCall{{Name: "usermod", Args: []string{"--gid", "operators", "--append", "--groups", "docker", "--", "alice"}}}
	if !reflect.DeepEqual(runner.Calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.Calls, want)
	}
}

// OS-LIA-002/003: all independently managed account attributes converge
// through a fixed usermod argv without treating omitted fields as drift.
func TestApplicator_AppliesManagedAccountAttributes(t *testing.T) {
	system := true
	a := users.New(models.UserResource{
		Name:                 "alice",
		Username:             "alice",
		Present:              true,
		UID:                  200,
		AllowUIDReassignment: true,
		Home:                 "/srv/alice",
		Shell:                "/bin/zsh",
		Comment:              "Alice Example",
		System:               &system,
	})
	a.LookupFunc = func(string) (*user.User, error) {
		return &user.User{Username: "alice", Uid: "1500", Gid: "1500", HomeDir: "/home/alice", Name: "Old Comment"}, nil
	}
	a.LookupShellFunc = func(string) (string, error) { return "/bin/bash", nil }
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"usermod [--uid 200 --home /srv/alice --shell /bin/zsh --comment Alice Example -- alice]": {},
	}}
	a.Runner = runner

	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []executil.MockCall{{Name: "usermod", Args: []string{"--uid", "200", "--home", "/srv/alice", "--shell", "/bin/zsh", "--comment", "Alice Example", "--", "alice"}}}
	if !reflect.DeepEqual(runner.Calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.Calls, want)
	}
}

// OS-LIA-004: authoritative mode replaces the owned supplementary set rather
// than appending to it.
func TestApplicator_AuthoritativeGroupMembershipsReplaceSet(t *testing.T) {
	a := users.New(models.UserResource{
		Name:                    "alice",
		Username:                "alice",
		Present:                 true,
		SupplementaryGroups:     []string{"docker"},
		SupplementaryGroupsMode: models.GroupMembershipAuthoritative,
		ResourceMeta:            models.ResourceMeta{Ownership: models.OwnershipAuthoritative},
	})
	a.LookupFunc = func(string) (*user.User, error) {
		return &user.User{Username: "alice", Uid: "1000", Gid: "1000"}, nil
	}
	a.LookupGroupFunc = func(name string) (*user.Group, error) { return &user.Group{Name: name, Gid: "3000"}, nil }
	a.GroupIDsFunc = func(*user.User) ([]string, error) { return []string{"1000", "2000"}, nil }
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"usermod [--groups docker -- alice]": {},
	}}
	a.Runner = runner

	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := runner.Calls; !reflect.DeepEqual(got, []executil.MockCall{{Name: "usermod", Args: []string{"--groups", "docker", "--", "alice"}}}) {
		t.Fatalf("calls = %#v", got)
	}
}

// OS-LIA-002/003: creation applies all declared account attributes with a
// fixed useradd argv, including an explicit home-creation policy.
func TestApplicator_CreatesAccountWithManagedAttributes(t *testing.T) {
	system, createHome := true, true
	a := users.New(models.UserResource{
		Name: "agent", Username: "agent", Present: true, UID: 200,
		PrimaryGroup: "operators", SupplementaryGroups: []string{"docker"}, SupplementaryGroupsMode: models.GroupMembershipMerge,
		Home: "/srv/agent", CreateHome: &createHome, Shell: "/usr/sbin/nologin", Comment: "Service Agent", System: &system,
	})
	a.LookupFunc = func(string) (*user.User, error) { return nil, errors.New("not found") }
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"useradd [--system --uid 200 --gid operators --groups docker --home /srv/agent --create-home --shell /usr/sbin/nologin --comment Service Agent -- agent]": {},
	}}
	a.Runner = runner

	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []executil.MockCall{{Name: "useradd", Args: []string{"--system", "--uid", "200", "--gid", "operators", "--groups", "docker", "--home", "/srv/agent", "--create-home", "--shell", "/usr/sbin/nologin", "--comment", "Service Agent", "--", "agent"}}}
	if !reflect.DeepEqual(runner.Calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.Calls, want)
	}
}

// OS-LIA-006: password hashes are resolved from a reference and sent only to
// chpasswd stdin; no password material appears in argv or runner telemetry.
func TestApplicator_PasswordReferenceUsesProtectedInput(t *testing.T) {
	secretPath := filepath.Join(t.TempDir(), "password-hash")
	const desiredHash = "$6$desired$opaque"
	if err := os.WriteFile(secretPath, []byte(desiredHash+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := users.New(models.UserResource{
		Name: "alice", Username: "alice", Present: true, PasswordHashRef: "file:" + secretPath,
	})
	a.LookupFunc = func(string) (*user.User, error) {
		return &user.User{Username: "alice", Uid: "1000", Gid: "1000"}, nil
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"getent [shadow alice]":  {Stdout: []byte("alice:$6$old$opaque:20000:0:99999:7:::\n")},
		"chpasswd [--encrypted]": {},
	}}
	a.Runner = runner

	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, call := range runner.Calls {
		if reflect.DeepEqual(call.Args, []string{desiredHash}) {
			t.Fatalf("password hash was passed as argv: %#v", call)
		}
	}
	if got := runner.Inputs; len(got) != 1 || got[0].Name != "chpasswd" || string(got[0].Input) != "alice:"+desiredHash+"\n" {
		t.Fatalf("protected input = %#v", got)
	}
}

// OS-LIA-005: removal never deletes the active Remotr runtime account, even
// when generic user deletion is otherwise configured.
func TestApplicator_BlocksRuntimeAccountRemoval(t *testing.T) {
	a := users.New(models.UserResource{Name: "agent", Username: "remotr-agent", Present: false})
	a.RuntimeUsername = "remotr-agent"
	called := false
	a.DelFunc = func(string) error { called = true; return nil }

	if err := a.Apply(context.Background()); err == nil {
		t.Fatal("expected runtime account removal to be blocked")
	}
	if called {
		t.Fatal("runtime account reached userdel")
	}
}

// OS-LIA-005: lock state and expiry are independently observed and converged
// with explicit usermod/chage argv.
func TestApplicator_AppliesLockAndExpiry(t *testing.T) {
	locked := true
	a := users.New(models.UserResource{
		Name: "alice", Username: "alice", Present: true, Locked: &locked, Expiry: "2030-01-02",
	})
	a.LookupFunc = func(string) (*user.User, error) {
		return &user.User{Username: "alice", Uid: "1000", Gid: "1000"}, nil
	}
	a.LockLookupFunc = func(string) (bool, error) { return false, nil }
	a.ExpiryLookupFunc = func(string) (string, error) { return "never", nil }
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"usermod [--lock -- alice]":                {},
		"chage [--expiredate 2030-01-02 -- alice]": {},
	}}
	a.Runner = runner

	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []executil.MockCall{
		{Name: "usermod", Args: []string{"--lock", "--", "alice"}},
		{Name: "chage", Args: []string{"--expiredate", "2030-01-02", "--", "alice"}},
	}
	if !reflect.DeepEqual(runner.Calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.Calls, want)
	}
}

// OS-LIA-005: destructive removal options are explicit and bounded to the
// named account through argv, rather than inferred from resource omission.
func TestApplicator_RemovesHomeOnlyWhenExplicitlyRequested(t *testing.T) {
	a := users.New(models.UserResource{
		Name: "obsolete", Username: "obsolete", Present: false, RemoveHome: true,
	})
	a.RuntimeUsername = "remotr-agent"
	a.ProtectedUserFunc = func(string) bool { return false }
	a.LookupFunc = func(string) (*user.User, error) { return &user.User{Username: "obsolete"}, nil }
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"userdel [--remove -- obsolete]": {},
	}}
	a.Runner = runner

	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := runner.Calls; !reflect.DeepEqual(got, []executil.MockCall{{Name: "userdel", Args: []string{"--remove", "--", "obsolete"}}}) {
		t.Fatalf("calls = %#v", got)
	}
}
