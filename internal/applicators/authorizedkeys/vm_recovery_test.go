//go:build vmsafety

package authorizedkeys_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/authorizedkeys"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

const vmAccessUser = "remotr-vm-access"

// OS-AEC-098: runs as root in the disposable Ubuntu access VM. It proves the
// real user-home boundary, merge and authoritative ownership, restrictions,
// expiry, revocation, recovery-principal preflight, symlink rejection, and
// provider-contract second Checks without replacing an unmanaged grant.
func TestAuthorizedKeyProviderContractVM(t *testing.T) {
	const (
		managedUser  = "remotr-vm-authorized-key"
		recoveryUser = "remotr-vm-authorized-recovery"
		unmanagedKey = "AAAAC3NzaC1lZDI1NTE5AAAAIMnQ2K0IuFmIVQDx53WZg0P6JiyMxX6M7BjWb3K4q3qQ"
	)
	vmRemoveNamedAccessUser(managedUser)
	vmRemoveNamedAccessUser(recoveryUser)
	if output, err := exec.Command("useradd", "--create-home", "--shell", "/bin/sh", "--", managedUser).CombinedOutput(); err != nil {
		t.Fatalf("create managed access user: %v: %s", err, output)
	}
	if output, err := exec.Command("useradd", "--create-home", "--shell", "/bin/sh", "--", recoveryUser).CombinedOutput(); err != nil {
		vmRemoveNamedAccessUser(managedUser)
		t.Fatalf("create recovery access user: %v: %s", err, output)
	}
	t.Cleanup(func() {
		vmRemoveNamedAccessUser(managedUser)
		vmRemoveNamedAccessUser(recoveryUser)
	})
	account, err := user.Lookup(managedUser)
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := strconv.Atoi(account.Uid)
	gid, _ := strconv.Atoi(account.Gid)
	sshDir := filepath.Join(account.HomeDir, ".ssh")
	if err := os.Mkdir(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(sshDir, uid, gid); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sshDir, "authorized_keys")
	unmanaged := []byte("ssh-ed25519 " + unmanagedKey + " unmanaged-before-remotr@example\n")
	if err := os.WriteFile(path, unmanaged, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, uid, gid); err != nil {
		t.Fatal(err)
	}

	resource := models.AuthorizedKeyResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipMerge},
		Name:         "qualified-access", User: managedUser,
		Entries: []models.AuthorizedKeyEntry{{
			Type: "ssh-ed25519", Key: administratorKey, Fingerprint: administratorFingerprint,
			Comment: "qualified access", Restrictions: []string{"no-agent-forwarding"},
			Principals: []string{"operator"}, ExpiresAt: "2037-01-02T03:04:05Z",
		}},
	}
	if err := resource.Validate(); err != nil {
		t.Fatal(err)
	}
	provider := newVMAuthorizedKeyProvider(t, resource)
	if result := provider.Check(context.Background()); result.Status != contract.Drifted {
		t.Fatalf("missing key Check = %+v, want drifted", result)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("merge Apply = %+v, want changed", result)
	}
	assertVMAuthorizedKeyCheck(t, provider, path, unmanaged, "no-agent-forwarding", `principals="operator"`, `expiry-time="20370102030405Z"`)
	if result := provider.Apply(context.Background()); result.Status != contract.NoChange || result.Err != nil {
		t.Fatalf("compliant merge Apply = %+v, want no change", result)
	}

	resource.Ownership = models.OwnershipAuthoritative
	resource.RecoveryPrincipals = []string{recoveryUser}
	resource.Entries[0].Restrictions = []string{"no-port-forwarding"}
	applicator := authorizedkeys.New(resource)
	if err := applicator.Preflight(context.Background()); err != nil {
		t.Fatalf("recovery-principal preflight: %v", err)
	}
	provider, err = contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("authoritative replacement Apply = %+v, want changed", result)
	}
	assertVMAuthorizedKeyCheck(t, provider, path, unmanaged, "no-port-forwarding")
	if output, err := exec.Command("su", "-s", "/bin/sh", "-c", "true", recoveryUser).CombinedOutput(); err != nil {
		t.Fatalf("recovery principal unusable: %v: %s", err, output)
	}

	resource.Lifecycle = models.LifecycleAbsent
	resource.Entries = nil
	provider = newVMAuthorizedKeyProvider(t, resource)
	if result := provider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("revocation Apply = %+v, want changed", result)
	}
	if result := provider.Check(context.Background()); result.Status != contract.Compliant {
		t.Fatalf("revocation second Check = %+v, want compliant", result)
	}
	content, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(content, unmanaged) {
		t.Fatalf("revocation content = %q, err=%v, want unmanaged grant", content, err)
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sshDir); err != nil {
		t.Fatal(err)
	}
	escape := t.TempDir()
	if err := os.Symlink(escape, sshDir); err != nil {
		t.Fatal(err)
	}
	resource.Lifecycle = models.LifecyclePresent
	resource.Ownership = models.OwnershipMerge
	resource.RecoveryPrincipals = nil
	resource.Entries = []models.AuthorizedKeyEntry{{Type: "ssh-ed25519", Key: administratorKey, Fingerprint: administratorFingerprint}}
	provider = newVMAuthorizedKeyProvider(t, resource)
	if result := provider.Apply(context.Background()); result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("symlinked home Apply = %+v, want failed", result)
	}
	if _, err := os.Stat(filepath.Join(escape, "authorized_keys")); !os.IsNotExist(err) {
		t.Fatalf("authorized key escaped managed home: %v", err)
	}
}

func newVMAuthorizedKeyProvider(t *testing.T, resource models.AuthorizedKeyResource) contract.Provider {
	t.Helper()
	provider, err := contract.New(authorizedkeys.New(resource))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func assertVMAuthorizedKeyCheck(t *testing.T, provider contract.Provider, path string, unmanaged []byte, fragments ...string) {
	t.Helper()
	if result := provider.Check(context.Background()); result.Status != contract.Compliant {
		t.Fatalf("authorized key second Check = %+v, want compliant", result)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(content, unmanaged) {
		t.Fatalf("authorized_keys did not preserve unmanaged grant: %q", content)
	}
	for _, fragment := range fragments {
		if !bytes.Contains(content, []byte(fragment)) {
			t.Fatalf("authorized_keys = %q, missing %q", content, fragment)
		}
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("authorized_keys mode = %v, err=%v", info.Mode().Perm(), err)
	}
}

// TestAuthorizedKeyInterruptedRecoveryVM runs in two processes separated by
// the harness's controlled Ubuntu reboot. It proves that an authoritative
// access change retains a real recovery principal, encrypts its exact prior
// file, and restores that file exactly once after provider reconstruction.
func TestAuthorizedKeyInterruptedRecoveryVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("VM access recovery test must run as root")
	}
	phase := os.Getenv("REMOTR_ACCESS_VM_PHASE")
	stateDir := os.Getenv("REMOTR_ACCESS_VM_STATE_DIR")
	if phase == "" || stateDir == "" {
		t.Fatal("REMOTR_ACCESS_VM_PHASE and REMOTR_ACCESS_VM_STATE_DIR are required")
	}

	ctx := context.Background()
	resource := models.AuthorizedKeyResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipAuthoritative},
		Name:         "vm-restart-access", User: vmAccessUser, RecoveryPrincipals: []string{"root"},
		Entries: []models.AuthorizedKeyEntry{{
			Type: "ssh-ed25519", Key: administratorKey, Fingerprint: administratorFingerprint,
			Comment: "remotr VM access",
		}},
	}
	rollbackRoot := filepath.Join(stateDir, "transactions")

	switch phase {
	case "prepare":
		vmRemoveAccessUser()
		if err := os.RemoveAll(stateDir); err != nil {
			t.Fatal(err)
		}
		if err := exec.Command("useradd", "--create-home", "--shell", "/bin/bash", vmAccessUser).Run(); err != nil {
			t.Fatalf("create disposable access user: %v", err)
		}
		account, err := user.Lookup(vmAccessUser)
		if err != nil {
			t.Fatal(err)
		}
		uid, _ := strconv.Atoi(account.Uid)
		gid, _ := strconv.Atoi(account.Gid)
		sshDir := filepath.Join(account.HomeDir, ".ssh")
		if err := os.MkdirAll(sshDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chown(sshDir, uid, gid); err != nil {
			t.Fatal(err)
		}
		original := vmOriginalAuthorizedKeys()
		path := filepath.Join(sshDir, "authorized_keys")
		if err := os.WriteFile(path, original, 0o640); err != nil {
			t.Fatal(err)
		}
		if err := os.Chown(path, uid, gid); err != nil {
			t.Fatal(err)
		}

		store := vmAccessRollbackStore(t, rollbackRoot)
		vmAssertRootFileProtection(t, store)
		provider := authorizedkeys.New(resource)
		if err := provider.ConfigureRollback(store, "authorizedKey.vm-restart-access", "sha256:vm-access"); err != nil {
			t.Fatal(err)
		}
		if err := provider.Preflight(ctx); err != nil {
			t.Fatalf("recovery-principal preflight: %v", err)
		}
		if err := provider.PreflightRollback(ctx); err != nil {
			t.Fatalf("protected rollback preflight: %v", err)
		}
		result := provider.ApplyResult(ctx)
		if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional || result.Err != nil {
			t.Fatalf("ApplyResult = %+v, want changed transactional", result)
		}
		if check := provider.Check(ctx); check.Status != executor.Compliant {
			t.Fatalf("post-Apply Check = %+v, want compliant", check)
		}
		vmAssertArmedSensitiveRecord(t, store, "authorizedKey.vm-restart-access")
		vmAssertTreeExcludes(t, rollbackRoot, original)
	case "verify":
		store := vmAccessRollbackStore(t, rollbackRoot)
		vmAssertRootFileProtection(t, store)
		provider := authorizedkeys.New(resource)
		if err := provider.ConfigureRollback(store, "authorizedKey.vm-restart-access", "sha256:vm-access"); err != nil {
			t.Fatal(err)
		}
		if err := provider.Preflight(ctx); err != nil {
			t.Fatalf("reconstructed recovery-principal preflight: %v", err)
		}
		if err := provider.Revert(ctx); err != nil {
			t.Fatalf("restart rollback: %v", err)
		}
		account, err := user.Lookup(vmAccessUser)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(account.HomeDir, ".ssh", "authorized_keys")
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, vmOriginalAuthorizedKeys()) {
			t.Fatalf("restored authorized_keys = %q, err=%v", got, err)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o640 {
			t.Fatalf("restored mode = %v, err=%v", info.Mode().Perm(), err)
		}
		if check := provider.Check(ctx); check.Status != executor.Drifted {
			t.Fatalf("second Check after rollback = %+v, want drifted", check)
		}
		if err := provider.Revert(ctx); !errors.Is(err, appErr.ErrNoOp) {
			t.Fatalf("second rollback = %v, want no replay", err)
		}
		vmAssertTreeExcludes(t, rollbackRoot, vmOriginalAuthorizedKeys())
		vmRemoveAccessUser()
		if err := os.RemoveAll(stateDir); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown VM access recovery phase %q", phase)
	}
}

func vmAccessRollbackStore(t *testing.T, root string) *rollbackstore.Store {
	t.Helper()
	store, err := rollbackstore.New(rollbackstore.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func vmAssertRootFileProtection(t *testing.T, store *rollbackstore.Store) {
	t.Helper()
	report := store.Protection()
	if report.Class != rollbackstore.ProtectionRootFile || !report.ReducedProtection || report.Limitation != rollbackstore.RootCompromiseLimitation || report.KeyID == "" {
		t.Fatalf("Ubuntu rollback protection report = %+v", report)
	}
}

func vmAssertArmedSensitiveRecord(t *testing.T, store *rollbackstore.Store, address string) {
	t.Helper()
	records, err := store.Records(context.Background(), address)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].State != rollbackstore.LifecycleArmed || !records[0].Armed || !records[0].Sensitive || !records[0].PayloadAvailable {
		t.Fatalf("protected access records = %+v", records)
	}
}

func vmOriginalAuthorizedKeys() []byte {
	return []byte("ssh-ed25519 " + administratorKey + " recovery-before-remotr@example\n")
}

func vmAssertTreeExcludes(t *testing.T, root string, secret []byte) {
	t.Helper()
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(raw, secret) {
			t.Fatalf("protected access payload appeared in %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func vmRemoveAccessUser() {
	vmRemoveNamedAccessUser(vmAccessUser)
}

func vmRemoveNamedAccessUser(username string) {
	if _, err := user.Lookup(username); err == nil {
		_ = exec.Command("userdel", "--remove", username).Run()
	}
}
