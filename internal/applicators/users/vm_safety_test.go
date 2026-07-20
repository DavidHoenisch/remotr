//go:build vmsafety

package users_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/users"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

// OS-AEC-098: runs only as root in the isolated Ubuntu 24.04 VM. It exercises
// the native passwd, group, and shadow databases through the public provider
// contract and retains an independently usable recovery principal throughout.
func TestUserProviderContractVM(t *testing.T) {
	const (
		managedUser   = "remotr-vm-qualified-user"
		recoveryUser  = "remotr-vm-qualified-recovery"
		primaryGroup  = "remotr-vm-primary"
		extraGroup    = "remotr-vm-extra"
		initialUID    = 27130
		updatedUID    = 27131
		primaryGID    = 27130
		extraGID      = 27131
		recoveryUID   = 27132
		recoveryGID   = 27132
		desiredHash   = "$6$remotr$qualificationhash"
		desiredExpiry = "2037-01-02"
	)
	home := filepath.Join("/home", managedUser)
	cleanupVMUser(managedUser)
	cleanupVMUser(recoveryUser)
	cleanupVMUserGroups(primaryGroup, extraGroup, recoveryUser)
	for _, group := range []struct {
		name string
		gid  int
	}{{primaryGroup, primaryGID}, {extraGroup, extraGID}, {recoveryUser, recoveryGID}} {
		runVMCommand(t, "groupadd", "--gid", fmt.Sprint(group.gid), "--", group.name)
	}
	runVMCommand(t, "useradd", "--uid", fmt.Sprint(recoveryUID), "--gid", recoveryUser, "--create-home", "--shell", "/bin/sh", "--", recoveryUser)
	t.Cleanup(func() {
		cleanupVMUser(managedUser)
		cleanupVMUser(recoveryUser)
		cleanupVMUserGroups(primaryGroup, extraGroup, recoveryUser)
	})

	secretPath := filepath.Join(t.TempDir(), "password-hash")
	if err := os.WriteFile(secretPath, []byte(desiredHash+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ordinary, createHome, locked := false, true, true
	resource := models.UserResource{
		Name: "qualified-user", Username: managedUser, Present: true, UID: initialUID,
		PrimaryGroup: primaryGroup, SupplementaryGroups: []string{extraGroup},
		SupplementaryGroupsMode: models.GroupMembershipAuthoritative,
		Home:                    home, CreateHome: &createHome, Shell: "/bin/sh", Comment: "Remotr Qualified User", System: &ordinary,
		PasswordHashRef: "file:" + secretPath, Locked: &locked, Expiry: desiredExpiry,
	}
	provider := newVMUserProvider(t, resource)
	if result := provider.Check(context.Background()); result.Status != contract.Drifted {
		t.Fatalf("missing user Check = %+v, want drifted", result)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("create user Apply = %+v, want changed", result)
	}
	assertVMUserCheck(t, provider, resource, desiredHash)
	if result := provider.Apply(context.Background()); result.Status != contract.NoChange || result.Err != nil {
		t.Fatalf("compliant user Apply = %+v, want no change", result)
	}

	resource.UID = updatedUID
	resource.AllowUIDReassignment = true
	provider = newVMUserProvider(t, resource)
	if result := provider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("UID reassignment Apply = %+v, want changed", result)
	}
	assertVMUserCheck(t, provider, resource, desiredHash)

	protected := users.New(models.UserResource{Name: "recovery", Username: recoveryUser, Present: false, RemoveHome: true})
	protected.RuntimeUsername = recoveryUser
	protected.ProtectedUserFunc = func(string) bool { return false }
	protectedProvider, err := contract.New(protected)
	if err != nil {
		t.Fatal(err)
	}
	if result := protectedProvider.Apply(context.Background()); result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("protected recovery removal Apply = %+v, want failed", result)
	}
	runVMCommand(t, "su", "-s", "/bin/sh", "-c", "true", recoveryUser)

	resource.Present = false
	resource.RemoveHome = true
	provider = newVMUserProvider(t, resource)
	if result := provider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("user removal Apply = %+v, want changed", result)
	}
	if result := provider.Check(context.Background()); result.Status != contract.Compliant {
		t.Fatalf("absent user second Check = %+v, want compliant", result)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("removed user home still exists or cannot be checked: %v", err)
	}
	runVMCommand(t, "su", "-s", "/bin/sh", "-c", "true", recoveryUser)
}

func newVMUserProvider(t *testing.T, resource models.UserResource) contract.Provider {
	t.Helper()
	applicator := users.New(resource)
	applicator.RuntimeUsername = "remotr-agent"
	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func assertVMUserCheck(t *testing.T, provider contract.Provider, resource models.UserResource, desiredHash string) {
	t.Helper()
	if result := provider.Check(context.Background()); result.Status != contract.Compliant {
		t.Fatalf("user second Check = %+v, want compliant", result)
	}
	account, err := user.Lookup(resource.Username)
	if err != nil {
		t.Fatal(err)
	}
	if account.Uid != fmt.Sprint(resource.UID) || account.HomeDir != resource.Home {
		t.Fatalf("native account = %+v, want uid=%d home=%s", account, resource.UID, resource.Home)
	}
	passwd := runVMCommand(t, "getent", "passwd", resource.Username)
	fields := strings.Split(strings.TrimSpace(passwd), ":")
	if len(fields) != 7 || fields[4] != resource.Comment || fields[6] != resource.Shell {
		t.Fatalf("passwd record = %q, want comment=%q shell=%q", passwd, resource.Comment, resource.Shell)
	}
	groups := strings.Fields(runVMCommand(t, "id", "--name", "--groups", resource.Username))
	if len(groups) != 2 || groups[0] != resource.PrimaryGroup || groups[1] != resource.SupplementaryGroups[0] {
		t.Fatalf("user groups = %v, want [%s %s]", groups, resource.PrimaryGroup, resource.SupplementaryGroups[0])
	}
	shadow := strings.Split(strings.TrimSpace(runVMCommand(t, "getent", "shadow", resource.Username)), ":")
	if len(shadow) < 2 || strings.TrimLeft(shadow[1], "!") != desiredHash || !strings.HasPrefix(shadow[1], "!") {
		t.Fatalf("shadow password did not preserve the declared hash and locked state")
	}
	if output := runVMCommand(t, "chage", "--list", "--iso8601", resource.Username); !strings.Contains(output, "Account expires") || !strings.Contains(output, resource.Expiry) {
		t.Fatalf("account expiry = %q, want %s", output, resource.Expiry)
	}
}

func runVMCommand(t *testing.T, name string, args ...string) string {
	t.Helper()
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, output)
	}
	return string(output)
}

func cleanupVMUser(name string) {
	_ = exec.Command("userdel", "--remove", "--force", "--", name).Run()
}

func cleanupVMUserGroups(names ...string) {
	for _, name := range names {
		_ = exec.Command("groupdel", "--", name).Run()
	}
}

// OS-LIA-005: runs only in the isolated Vagrant VM. It proves a real userdel
// can remove a disposable account while a separate recovery account remains
// usable, and that the provider blocks deletion of its runtime identity before
// it invokes userdel.
func TestUserRemovalSafetyVM(t *testing.T) {
	if err := exec.Command("useradd", "--create-home", "remotr-vm-target").Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("useradd", "--create-home", "remotr-vm-recovery").Run(); err != nil {
		_ = exec.Command("userdel", "--remove", "remotr-vm-target").Run()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = exec.Command("userdel", "--remove", "remotr-vm-target").Run()
		_ = exec.Command("userdel", "--remove", "remotr-vm-recovery").Run()
	})

	provider := users.New(models.UserResource{Name: "target", Username: "remotr-vm-target", Present: false, RemoveHome: true})
	provider.RuntimeUsername = "remotr-agent"
	provider.ProtectedUserFunc = func(string) bool { return false }
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := user.Lookup("remotr-vm-target"); err == nil {
		t.Fatal("target account remains after explicit removal")
	}
	if _, err := user.Lookup("remotr-vm-recovery"); err != nil {
		t.Fatalf("recovery account was disturbed: %v", err)
	}

	runtime := users.New(models.UserResource{Name: "recovery", Username: "remotr-vm-recovery", Present: false, RemoveHome: true})
	runtime.RuntimeUsername = "remotr-vm-recovery"
	runtime.ProtectedUserFunc = func(string) bool { return false }
	if err := runtime.Apply(context.Background()); err == nil {
		t.Fatal("runtime identity removal was not blocked")
	}
	if _, err := user.Lookup("remotr-vm-recovery"); err != nil {
		t.Fatalf("runtime account was removed: %v", err)
	}
}
