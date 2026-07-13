//go:build vmsafety

package users_test

import (
	"context"
	"os/exec"
	"os/user"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/users"
	"github.com/DavidHoenisch/remotr/internal/models"
)

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
