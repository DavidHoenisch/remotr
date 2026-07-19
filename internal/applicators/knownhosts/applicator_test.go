package knownhosts_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/knownhosts"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

const hostKey = "AAAAC3NzaC1lZDI1NTE5AAAAIPTCEW4tXxI1a3nVVLmEEu2WADFX6GeP0HeZg2N5DR9W"
const hostFingerprint = "SHA256:YX/1T3lbmFP3mL3tZEfnRA79p12FyzmdPJnh4P7TLd4"

// OS-LIA-009: a named known-host entry preserves unrelated lines and refuses
// to replace a conflicting host key until replacement is explicitly allowed.
func TestKnownHostApplicatorPreservesUnrelatedEntriesAndGatesReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh_known_hosts")
	conflicting := "git.example ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMnQ2K0IuFmIVQDx53WZg0P6JiyMxX6M7BjWb3K4q3qQ stale\n"
	unmanaged := "build.example ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMnQ2K0IuFmIVQDx53WZg0P6JiyMxX6M7BjWb3K4q3qQ build\n"
	if err := os.WriteFile(path, []byte(conflicting+unmanaged), 0o644); err != nil {
		t.Fatal(err)
	}
	resource := models.KnownHostResource{
		Name: "git-host", Scope: models.KnownHostScopeSystem, Hosts: []string{"git.example"},
		Type: "ssh-ed25519", Key: hostKey, Fingerprint: hostFingerprint, Hashing: models.KnownHostHashPlain,
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipNamed},
	}
	provider := knownhosts.New(resource)
	provider.SystemPath = path

	if err := provider.Apply(context.Background()); err == nil {
		t.Fatal("conflicting host key must require replaceExisting")
	}
	provider.Resource.ReplaceExisting = true
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		unmanaged,
		"# >>> remotr known_hosts git-host >>>",
		"git.example ssh-ed25519 " + hostKey,
		"# <<< remotr known_hosts git-host <<<",
	} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("known_hosts = %q, want %q", content, want)
		}
	}
	if strings.Contains(string(content), conflicting) {
		t.Fatalf("replaceExisting left conflicting host key: %q", content)
	}
	if _, compliant := provider.State(context.Background()); !compliant {
		t.Fatal("managed known-host entry must be compliant after Apply")
	}
}

// OS-LIA-009: a hash policy stores OpenSSH hashed host patterns but still
// recognizes the managed host on subsequent checks.
func TestKnownHostApplicatorChecksHashedHostPattern(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh_known_hosts")
	provider := knownhosts.New(models.KnownHostResource{
		Name: "hashed-git", Scope: models.KnownHostScopeSystem, Hosts: []string{"git.example"},
		Type: "ssh-ed25519", Key: hostKey, Fingerprint: hostFingerprint, Hashing: models.KnownHostHashHashed,
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipNamed},
	})
	provider.SystemPath = path
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "|1|") || strings.Contains(string(content), "git.example ssh-ed25519") {
		t.Fatalf("known_hosts hash policy = %q", content)
	}
	if _, compliant := provider.State(context.Background()); !compliant {
		t.Fatal("hashed managed host must be recognized as compliant")
	}
}

// OS-AEC-080: host-trust rollback survives an agent restart and restores the
// exact system known_hosts state before another mutation is admitted.
func TestKnownHostApplicatorRestoresProtectedStateAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh_known_hosts")
	original := []byte("build.example ssh-ed25519 unmanaged-before-remotr build@example\n")
	if err := os.WriteFile(path, original, 0o640); err != nil {
		t.Fatal(err)
	}
	resource := models.KnownHostResource{
		Name: "restart-host", Scope: models.KnownHostScopeSystem, Hosts: []string{"git.example"},
		Type: "ssh-ed25519", Key: hostKey, Fingerprint: hostFingerprint, Hashing: models.KnownHostHashPlain,
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipNamed},
	}
	storeRoot := filepath.Join(t.TempDir(), "transactions")
	store, err := rollbackstore.New(rollbackstore.Options{Root: storeRoot})
	if err != nil {
		t.Fatal(err)
	}
	provider := knownhosts.New(resource)
	provider.SystemPath = path
	if err := provider.ConfigureRollback(store, "knownHost.restart-host", "artifact-a"); err != nil {
		t.Fatal(err)
	}
	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional || result.Err != nil {
		t.Fatalf("ApplyResult = %+v, want changed transactional", result)
	}
	records, err := store.Records(context.Background(), "knownHost.restart-host")
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
	restarted := knownhosts.New(resource)
	restarted.SystemPath = path
	if err := restarted.ConfigureRollback(restartedStore, "knownHost.restart-host", "artifact-a"); err != nil {
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
		t.Fatalf("known_hosts after restart rollback = %q, want %q", got, original)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("known_hosts mode after restart rollback = %04o, want 0640", info.Mode().Perm())
	}
}
