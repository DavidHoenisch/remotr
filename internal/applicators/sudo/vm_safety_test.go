//go:build vmsafety

package sudo_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/sudo"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

// OS-AEC-098: runs as root in the disposable Ubuntu access VM. It validates a
// complete staged sudoers tree with native visudo, exercises real grants for a
// managed and recovery principal, rejects an invalid candidate without active
// mutation, rolls back, and proves the recovery path remains usable.
func TestSudoProviderContractVM(t *testing.T) {
	const (
		managedUser      = "remotr-vm-sudo-target"
		recoveryUser     = "remotr-vm-sudo-recovery"
		managedFragment  = "/etc/sudoers.d/remotr-vm-qualified"
		recoveryFragment = "/etc/sudoers.d/remotr-vm-recovery"
	)
	if os.Geteuid() != 0 {
		t.Fatal("sudo VM contract must run as root")
	}
	vmRemoveSudoUser(managedUser)
	vmRemoveSudoUser(recoveryUser)
	_ = os.Remove(managedFragment)
	_ = os.Remove(recoveryFragment)
	for _, username := range []string{managedUser, recoveryUser} {
		if output, err := exec.Command("useradd", "--create-home", "--shell", "/bin/sh", "--", username).CombinedOutput(); err != nil {
			t.Fatalf("create %s: %v: %s", username, err, output)
		}
	}
	t.Cleanup(func() {
		_ = os.Remove(managedFragment)
		_ = os.Remove(recoveryFragment)
		vmRemoveSudoUser(managedUser)
		vmRemoveSudoUser(recoveryUser)
	})
	recoveryPolicy := []byte(recoveryUser + " ALL=(root) NOPASSWD: /usr/bin/id\n")
	if err := os.WriteFile(recoveryFragment, recoveryPolicy, 0o440); err != nil {
		t.Fatal(err)
	}
	vmRunSudoCommand(t, "visudo", "-cf", "/etc/sudoers")
	vmAssertSudoGrant(t, recoveryUser)

	resource := models.SudoResource{
		Name: "remotr-vm-qualified", Subjects: []string{managedUser}, RunAs: []string{"root"},
		Commands: []string{"/usr/bin/id"}, Tags: []string{"NOPASSWD"}, RecoveryPrincipals: []string{recoveryUser},
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipFragment},
	}
	store, err := rollbackstore.New(rollbackstore.Options{Root: filepath.Join(t.TempDir(), "transactions")})
	if err != nil {
		t.Fatal(err)
	}
	applicator := sudo.New(resource)
	if err := applicator.ConfigureRollback(store, "sudo.remotr-vm-qualified", "sha256:vm-sudo"); err != nil {
		t.Fatal(err)
	}
	if err := applicator.Preflight(context.Background()); err != nil {
		t.Fatalf("recovery preflight: %v", err)
	}
	if err := applicator.PreflightRollback(context.Background()); err != nil {
		t.Fatalf("rollback preflight: %v", err)
	}
	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}
	if result := provider.Check(context.Background()); result.Status != contract.Drifted {
		t.Fatalf("missing sudo Check = %+v, want drifted", result)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.Changed || result.RollbackClass != contract.RollbackTransactional || result.Err != nil {
		t.Fatalf("sudo Apply = %+v, want changed transactional", result)
	}
	if result := provider.Check(context.Background()); result.Status != contract.Compliant {
		t.Fatalf("sudo second Check = %+v, want compliant", result)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.NoChange || result.Err != nil {
		t.Fatalf("compliant sudo Apply = %+v, want no change", result)
	}
	vmRunSudoCommand(t, "visudo", "-cf", "/etc/sudoers")
	vmAssertSudoGrant(t, managedUser)
	vmAssertSudoGrant(t, recoveryUser)
	info, err := os.Stat(managedFragment)
	if err != nil || info.Mode().Perm() != 0o440 {
		t.Fatalf("managed fragment mode = %v, err=%v", info.Mode().Perm(), err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || stat.Gid != 0 {
		t.Fatalf("managed fragment ownership = %T %+v, want root:root", info.Sys(), stat)
	}
	active, err := os.ReadFile(managedFragment)
	if err != nil {
		t.Fatal(err)
	}

	invalid := resource
	invalid.Subjects = []string{"invalid subject"}
	invalidProvider, err := contract.New(sudo.New(invalid))
	if err != nil {
		t.Fatal(err)
	}
	result := invalidProvider.Apply(context.Background())
	if result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("invalid effective policy Apply = %+v, want failed", result)
	}
	if strings.Contains(result.Err.Error(), "invalid subject") {
		t.Fatalf("invalid policy leaked through diagnostic: %v", result.Err)
	}
	got, err := os.ReadFile(managedFragment)
	if err != nil || !bytes.Equal(got, active) {
		t.Fatalf("invalid candidate changed active sudo policy: %q, err=%v", got, err)
	}
	vmAssertSudoGrant(t, recoveryUser)

	if result := provider.Rollback(context.Background()); result.Status != contract.Reverted || result.Err != nil {
		t.Fatalf("sudo rollback = %+v, want reverted", result)
	}
	if _, err := os.Stat(managedFragment); !os.IsNotExist(err) {
		t.Fatalf("rollback did not remove newly created fragment: %v", err)
	}
	if result := provider.Check(context.Background()); result.Status != contract.Drifted {
		t.Fatalf("post-rollback Check = %+v, want drifted", result)
	}
	vmRunSudoCommand(t, "visudo", "-cf", "/etc/sudoers")
	vmAssertSudoGrant(t, recoveryUser)
}

func vmAssertSudoGrant(t *testing.T, username string) {
	t.Helper()
	output := vmRunSudoCommand(t, "su", "-s", "/bin/sh", "-c", "sudo -n /usr/bin/id -u", username)
	if strings.TrimSpace(output) != "0" {
		t.Fatalf("sudo grant for %s returned %q, want root uid", username, output)
	}
}

func vmRunSudoCommand(t *testing.T, name string, args ...string) string {
	t.Helper()
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, output)
	}
	return string(output)
}

func vmRemoveSudoUser(username string) {
	if _, err := user.Lookup(username); err == nil {
		_ = exec.Command("userdel", "--remove", "--force", "--", username).Run()
	}
}
