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
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

const vmAccessUser = "remotr-vm-access"

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
	if _, err := user.Lookup(vmAccessUser); err == nil {
		_ = exec.Command("userdel", "--remove", vmAccessUser).Run()
	}
}
