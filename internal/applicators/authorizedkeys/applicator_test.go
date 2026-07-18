package authorizedkeys_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/authorizedkeys"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

const administratorKey = "AAAAC3NzaC1lZDI1NTE5AAAAIPTCEW4tXxI1a3nVVLmEEu2WADFX6GeP0HeZg2N5DR9W"
const administratorFingerprint = "SHA256:YX/1T3lbmFP3mL3tZEfnRA79p12FyzmdPJnh4P7TLd4"

// OS-LIA-007/008: a structured key set writes canonical restrictions inside
// its owned boundary, and later revocation never changes an unmanaged key.
func TestAuthorizedKeyApplicatorRevokesOnlyItsOwnedKeySet(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.Mkdir(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sshDir, "authorized_keys")
	unmanaged := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMnQ2K0IuFmIVQDx53WZg0P6JiyMxX6M7BjWb3K4q3qQ existing@example\n"
	if err := os.WriteFile(path, []byte(unmanaged), 0o600); err != nil {
		t.Fatal(err)
	}

	resource := models.AuthorizedKeyResource{
		Name: "admin-access",
		User: "admin",
		Entries: []models.AuthorizedKeyEntry{{
			Type:         "ssh-ed25519",
			Key:          administratorKey,
			Fingerprint:  administratorFingerprint,
			Comment:      "remotr administrator",
			Restrictions: []string{"from=\"10.0.0.0/8\"", "no-agent-forwarding"},
		}},
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipAuthoritative},
	}
	provider := authorizedkeys.New(resource)
	provider.LookupUser = func(string) (interactiveuser.Account, error) {
		return interactiveuser.Account{Username: "admin", UID: os.Getuid(), GID: os.Getgid(), HomeDir: home}, nil
	}

	if _, compliant := provider.State(context.Background()); compliant {
		t.Fatal("missing managed key set must drift")
	}
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		unmanaged,
		"# >>> remotr authorized_keys admin-access >>>",
		"from=\"10.0.0.0/8\",no-agent-forwarding ssh-ed25519 " + administratorKey + " remotr administrator",
		"# <<< remotr authorized_keys admin-access <<<",
	} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("authorized_keys = %q, want %q", content, want)
		}
	}
	if _, compliant := provider.State(context.Background()); !compliant {
		t.Fatal("managed key set must be compliant after Apply")
	}

	provider.Resource.Entries = nil
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != unmanaged {
		t.Fatalf("revocation changed unmanaged entries: %q", content)
	}
}

// OS-LIA-007: key management must not follow a user-controlled .ssh symlink
// outside the selected account home.
func TestAuthorizedKeyApplicatorRejectsSymlinkedSSHDirectory(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(home, ".ssh")); err != nil {
		t.Fatal(err)
	}
	provider := authorizedkeys.New(models.AuthorizedKeyResource{
		Name: "admin-access", User: "admin",
		Entries:      []models.AuthorizedKeyEntry{{Type: "ssh-ed25519", Key: administratorKey, Fingerprint: administratorFingerprint}},
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipMerge},
	})
	provider.LookupUser = func(string) (interactiveuser.Account, error) {
		return interactiveuser.Account{Username: "admin", UID: os.Getuid(), GID: os.Getgid(), HomeDir: home}, nil
	}

	if err := provider.Apply(context.Background()); err == nil {
		t.Fatal("expected unsafe .ssh symlink to be rejected")
	}
	if _, err := os.Stat(filepath.Join(outside, "authorized_keys")); !os.IsNotExist(err) {
		t.Fatalf("managed key escaped selected home: %v", err)
	}
}

// OS-LIA-007: merge ownership only adds newly declared entries. It never
// treats an omitted entry as permission to revoke an existing managed key.
func TestAuthorizedKeyApplicatorMergePreservesPriorManagedEntries(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	provider := authorizedkeys.New(models.AuthorizedKeyResource{
		Name: "team-access", User: "admin",
		Entries:      []models.AuthorizedKeyEntry{{Type: "ssh-ed25519", Key: administratorKey, Fingerprint: administratorFingerprint}},
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipMerge},
	})
	provider.LookupUser = func(string) (interactiveuser.Account, error) {
		return interactiveuser.Account{Username: "admin", UID: os.Getuid(), GID: os.Getgid(), HomeDir: home}, nil
	}
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}

	provider.Resource.Entries = nil
	if _, compliant := provider.State(context.Background()); !compliant {
		t.Fatal("merge ownership must preserve its prior managed entries")
	}
	if err := provider.Apply(context.Background()); err == nil {
		t.Fatal("merge set with no newly declared keys must remain compliant")
	}
	content, err := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), administratorKey) {
		t.Fatalf("merge ownership revoked prior managed entry: %q", content)
	}
}

// OS-LIA-008: an authoritative revocation also converges when the resource
// owns the entire file content, rather than requiring an unmanaged line to
// make the underlying file writer notice the replacement.
func TestAuthorizedKeyApplicatorRevokesItsOnlyManagedBlock(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	provider := authorizedkeys.New(models.AuthorizedKeyResource{
		Name: "sole-access", User: "admin",
		Entries:      []models.AuthorizedKeyEntry{{Type: "ssh-ed25519", Key: administratorKey, Fingerprint: administratorFingerprint}},
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipAuthoritative},
	})
	provider.LookupUser = func(string) (interactiveuser.Account, error) {
		return interactiveuser.Account{Username: "admin", UID: os.Getuid(), GID: os.Getgid(), HomeDir: home}, nil
	}
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.Resource.Lifecycle, provider.Resource.Entries = models.LifecycleAbsent, nil
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	if err != nil || strings.TrimSpace(string(content)) != "" {
		t.Fatalf("revoked-only key file = %q, %v", content, err)
	}
	if _, compliant := provider.State(context.Background()); !compliant {
		t.Fatal("revoked-only key file must be compliant")
	}
}

// OS-LIA-011: authoritative SSH changes need a real recovery-principal
// preflight before the engine permits access-risk enforcement.
func TestAuthorizedKeyApplicatorPreflightRequiresRecoveryPrincipal(t *testing.T) {
	provider := authorizedkeys.New(models.AuthorizedKeyResource{
		Name: "admin-access", User: "admin", RecoveryPrincipals: []string{"recovery"},
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipAuthoritative},
	})
	provider.RecoveryCheck = func(string) error { return os.ErrNotExist }
	if err := provider.Preflight(context.Background()); err == nil {
		t.Fatal("missing recovery principal must block authoritative SSH access changes")
	}
}

// OS-AEC-080: access rollback survives an agent restart and restores the
// descriptor-safely captured authorized_keys file before another mutation.
func TestAuthorizedKeyApplicatorRestoresProtectedStateAfterRestart(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.Mkdir(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sshDir, "authorized_keys")
	original := []byte("ssh-ed25519 unmanaged-before-remotr recovery@example\n")
	if err := os.WriteFile(path, original, 0o640); err != nil {
		t.Fatal(err)
	}
	resource := models.AuthorizedKeyResource{
		Name: "restart-access", User: "admin",
		Entries: []models.AuthorizedKeyEntry{{
			Type: "ssh-ed25519", Key: administratorKey, Fingerprint: administratorFingerprint,
		}},
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipMerge},
	}
	lookup := func(string) (interactiveuser.Account, error) {
		return interactiveuser.Account{Username: "admin", UID: os.Getuid(), GID: os.Getgid(), HomeDir: home}, nil
	}
	storeRoot := filepath.Join(t.TempDir(), "transactions")
	store, err := rollbackstore.New(rollbackstore.Options{Root: storeRoot})
	if err != nil {
		t.Fatal(err)
	}
	provider := authorizedkeys.New(resource)
	provider.LookupUser = lookup
	if err := provider.ConfigureRollback(store, "authorizedKey.restart-access", "artifact-a"); err != nil {
		t.Fatal(err)
	}
	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional || result.Err != nil {
		t.Fatalf("ApplyResult = %+v, want changed transactional", result)
	}
	records, err := store.Records(context.Background(), "authorizedKey.restart-access")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || !records[0].Armed || !records[0].Sensitive {
		t.Fatalf("rollback records = %+v, want one armed sensitive record", records)
	}

	restartedStore, err := rollbackstore.New(rollbackstore.Options{Root: storeRoot})
	if err != nil {
		t.Fatal(err)
	}
	restarted := authorizedkeys.New(resource)
	restarted.LookupUser = lookup
	if err := restarted.ConfigureRollback(restartedStore, "authorizedKey.restart-access", "artifact-a"); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Revert(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("authorized_keys after restart rollback = %q, want %q", got, original)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("authorized_keys mode after restart rollback = %04o, want 0640", info.Mode().Perm())
	}
}
