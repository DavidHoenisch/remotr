//go:build vmsafety

package userfiles_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/userfiles"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

// OS-AEC-098: runs as root in the disposable Ubuntu access VM. It proves real
// passwd-derived interactive-user resolution, per-user observations, content
// and metadata convergence, absence, no-follow home traversal, preservation,
// one-user failure isolation, idempotent Apply, and compliant second Checks.
func TestUserFileProviderContractVM(t *testing.T) {
	const (
		unsafeUser = "remotr-vm-userfile-unsafe"
		safeUser   = "remotr-vm-userfile-safe"
		content    = "managed=true\n"
	)
	if os.Geteuid() != 0 {
		t.Fatal("user-file VM contract must run as root")
	}
	unsafeAccount := vmCreateUserFileUser(t, unsafeUser)
	safeAccount := vmCreateUserFileUser(t, safeUser)
	accounts := []vmUserFileAccount{unsafeAccount, safeAccount}
	for _, account := range accounts {
		unmanagedPath := filepath.Join(account.home, "unmanaged-before-remotr")
		if err := os.WriteFile(unmanagedPath, []byte("preserve\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chown(unmanagedPath, account.uid, account.gid); err != nil {
			t.Fatal(err)
		}
	}

	resource := models.UserFileResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipMerge},
		Name:         "qualified-policy",
		Selector: &models.InteractiveUserSelector{
			Mode:      models.InteractiveUserSelectionExplicit,
			Usernames: []string{unsafeUser, safeUser},
		},
		Path:    ".config/remotr/policy.conf",
		Content: content,
		Mode:    []int{0o640},
	}
	if err := resource.Validate(); err != nil {
		t.Fatal(err)
	}
	provider := vmUserFileProvider(t, resource)
	check := provider.Check(context.Background())
	if check.Status != contract.Drifted || len(check.Subresults) != 2 {
		t.Fatalf("missing user files Check = %+v, want two drifted per-user results", check)
	}
	if check.Subresults[0].Target != unsafeUser || check.Subresults[0].Status != executor.Drifted ||
		check.Subresults[1].Target != safeUser || check.Subresults[1].Status != executor.Drifted {
		t.Fatalf("authored per-user Check order = %+v", check.Subresults)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("user-file Apply = %+v, want changed", result)
	}
	if result := provider.Check(context.Background()); result.Status != contract.Compliant || len(result.Subresults) != 2 {
		t.Fatalf("user-file second Check = %+v, want compliant per-user results", result)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.NoChange || result.Err != nil {
		t.Fatalf("compliant user-file Apply = %+v, want no change", result)
	}
	for _, account := range accounts {
		path := filepath.Join(account.home, resource.Path)
		vmAssertUserFile(t, account, path, []byte(content), 0o640)
		output, err := exec.Command("runuser", "-u", account.username, "--", "cat", path).CombinedOutput()
		if err != nil || !bytes.Equal(output, []byte(content)) {
			t.Fatalf("%s cannot read managed user file: %v: %s", account.username, err, output)
		}
	}

	safePath := filepath.Join(safeAccount.home, resource.Path)
	if err := os.Chmod(safePath, 0o600); err != nil {
		t.Fatal(err)
	}
	check = provider.Check(context.Background())
	if check.Status != contract.Drifted || len(check.Subresults) != 2 || check.Subresults[1].Status != executor.Drifted {
		t.Fatalf("metadata-only drift Check = %+v, want safe user drifted", check)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("metadata repair Apply = %+v, want changed", result)
	}
	if result := provider.Check(context.Background()); result.Status != contract.Compliant {
		t.Fatalf("metadata repair second Check = %+v, want compliant", result)
	}

	absent := resource
	absent.Lifecycle = models.LifecycleAbsent
	absent.Content = ""
	absent.Mode = nil
	if err := absent.Validate(); err != nil {
		t.Fatal(err)
	}
	absentProvider := vmUserFileProvider(t, absent)
	if result := absentProvider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("user-file absence Apply = %+v, want changed", result)
	}
	if result := absentProvider.Check(context.Background()); result.Status != contract.Compliant {
		t.Fatalf("user-file absence second Check = %+v, want compliant", result)
	}
	if result := absentProvider.Apply(context.Background()); result.Status != contract.NoChange || result.Err != nil {
		t.Fatalf("compliant absence Apply = %+v, want no change", result)
	}
	for _, account := range accounts {
		if _, err := os.Lstat(filepath.Join(account.home, resource.Path)); !os.IsNotExist(err) {
			t.Fatalf("%s managed user file remains after absence: %v", account.username, err)
		}
		unmanaged, err := os.ReadFile(filepath.Join(account.home, "unmanaged-before-remotr"))
		if err != nil || !bytes.Equal(unmanaged, []byte("preserve\n")) {
			t.Fatalf("%s unmanaged file changed: %q, err=%v", account.username, unmanaged, err)
		}
	}

	escape := t.TempDir()
	unsafeParent := filepath.Join(unsafeAccount.home, ".remotr-isolation")
	if err := os.Symlink(escape, unsafeParent); err != nil {
		t.Fatal(err)
	}
	isolation := resource
	isolation.Name = "qualified-isolation"
	isolation.Path = ".remotr-isolation/policy.conf"
	applicator := userfiles.New(isolation)
	if err := applicator.Apply(context.Background()); err == nil {
		t.Fatal("unsafe first user's symlinked parent Apply succeeded, want isolated failure")
	}
	if _, err := os.Lstat(filepath.Join(escape, "policy.conf")); !os.IsNotExist(err) {
		t.Fatalf("user-file escaped unsafe home: %v", err)
	}
	vmAssertUserFile(t, safeAccount, filepath.Join(safeAccount.home, isolation.Path), []byte(content), 0o640)
}

type vmUserFileAccount struct {
	username string
	home     string
	uid      int
	gid      int
}

func vmCreateUserFileUser(t *testing.T, username string) vmUserFileAccount {
	t.Helper()
	vmRemoveUserFileUser(username)
	if output, err := exec.Command("useradd", "--create-home", "--shell", "/bin/sh", "--", username).CombinedOutput(); err != nil {
		t.Fatalf("create %s: %v: %s", username, err, output)
	}
	t.Cleanup(func() { vmRemoveUserFileUser(username) })
	account, err := user.Lookup(username)
	if err != nil {
		t.Fatal(err)
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil {
		t.Fatal(err)
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil {
		t.Fatal(err)
	}
	return vmUserFileAccount{username: username, home: account.HomeDir, uid: uid, gid: gid}
}

func vmRemoveUserFileUser(username string) {
	if _, err := user.Lookup(username); err == nil {
		_ = exec.Command("userdel", "--remove", "--force", "--", username).Run()
	}
}

func vmUserFileProvider(t *testing.T, resource models.UserFileResource) contract.Provider {
	t.Helper()
	provider, err := contract.New(userfiles.New(resource))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func vmAssertUserFile(t *testing.T, account vmUserFileAccount, path string, content []byte, mode os.FileMode) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("%s content = %q, err=%v", path, got, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != mode {
		t.Fatalf("%s mode = %v, err=%v, want %v", path, info.Mode().Perm(), err, mode)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != account.uid || int(stat.Gid) != account.gid {
		t.Fatalf("%s ownership = %T %+v, want %d:%d", path, info.Sys(), stat, account.uid, account.gid)
	}
}
