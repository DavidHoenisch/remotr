//go:build vmsafety

package groups_test

import (
	"context"
	"os/exec"
	"os/user"
	"strconv"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/groups"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

// OS-AEC-098: runs only as root in the isolated Ubuntu 24.04 VM. It exercises
// the native group database through the public provider contract, including a
// fixed ordinary class, explicit GID reassignment, removal, and second Checks.
func TestGroupProviderContractVM(t *testing.T) {
	const (
		managedGroup  = "remotr-vm-qualified-group"
		recoveryGroup = "remotr-vm-recovery-group"
		initialGID    = 27120
		updatedGID    = 27121
	)
	cleanupVMGroup(managedGroup)
	cleanupVMGroup(recoveryGroup)
	if output, err := exec.Command("groupadd", "--gid", "27122", "--", recoveryGroup).CombinedOutput(); err != nil {
		t.Fatalf("create recovery group: %v: %s", err, output)
	}
	t.Cleanup(func() {
		cleanupVMGroup(managedGroup)
		cleanupVMGroup(recoveryGroup)
	})

	ordinary := false
	resource := models.GroupResource{
		Name: "qualified-group", Group: managedGroup, GID: initialGID, System: &ordinary,
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
	}
	provider := newVMGroupProvider(t, resource)
	if result := provider.Check(context.Background()); result.Status != contract.Drifted {
		t.Fatalf("missing group Check = %+v, want drifted", result)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("create group Apply = %+v, want changed", result)
	}
	assertVMGroupCheck(t, provider, managedGroup, initialGID)
	if result := provider.Apply(context.Background()); result.Status != contract.NoChange || result.Err != nil {
		t.Fatalf("compliant group Apply = %+v, want no change", result)
	}

	resource.GID = updatedGID
	resource.AllowGIDReassignment = true
	provider = newVMGroupProvider(t, resource)
	if result := provider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("GID reassignment Apply = %+v, want changed", result)
	}
	assertVMGroupCheck(t, provider, managedGroup, updatedGID)

	resource.Lifecycle = models.LifecycleAbsent
	provider = newVMGroupProvider(t, resource)
	if result := provider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("group removal Apply = %+v, want changed", result)
	}
	if result := provider.Check(context.Background()); result.Status != contract.Compliant {
		t.Fatalf("absent group second Check = %+v, want compliant", result)
	}
	if _, err := user.LookupGroup(recoveryGroup); err != nil {
		t.Fatalf("recovery group was disturbed: %v", err)
	}
}

func newVMGroupProvider(t *testing.T, resource models.GroupResource) contract.Provider {
	t.Helper()
	provider, err := contract.New(groups.New(resource, nil))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func assertVMGroupCheck(t *testing.T, provider contract.Provider, name string, gid int) {
	t.Helper()
	if result := provider.Check(context.Background()); result.Status != contract.Compliant {
		t.Fatalf("group second Check = %+v, want compliant", result)
	}
	group, err := user.LookupGroup(name)
	if err != nil {
		t.Fatal(err)
	}
	if group.Gid != strconv.Itoa(gid) {
		t.Fatalf("group gid = %s, want %d", group.Gid, gid)
	}
}

func cleanupVMGroup(name string) {
	_ = exec.Command("groupdel", "--", name).Run()
}
