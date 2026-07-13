package knownhosts_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/knownhosts"
	"github.com/DavidHoenisch/remotr/internal/models"
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
