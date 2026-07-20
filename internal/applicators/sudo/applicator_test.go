package sudo_test

import (
	"context"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/sudo"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

func testSudoResource() models.SudoResource {
	return models.SudoResource{
		Name:               "developer-admin",
		Subjects:           []string{"developer"},
		RunAs:              []string{"ALL"},
		Commands:           []string{"/usr/bin/id"},
		Tags:               []string{"NOPASSWD"},
		RecoveryPrincipals: []string{"recovery"},
		ResourceMeta:       models.ResourceMeta{Lifecycle: models.LifecyclePresent, Ownership: models.OwnershipFragment},
	}
}

// OS-LIA-010: failed effective sudoers validation leaves the active named
// fragment untouched; successful validation activates only that fragment.
func TestSudoApplicatorValidatesStagedEffectiveConfigurationBeforeActivation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "developer-admin")
	if err := os.WriteFile(path, []byte("old sudo policy\n"), 0o440); err != nil {
		t.Fatal(err)
	}
	sudoers := filepath.Join(dir, "sudoers")
	if err := os.WriteFile(sudoers, []byte("#includedir "+dir+"\n"), 0o440); err != nil {
		t.Fatal(err)
	}
	provider := sudo.New(testSudoResource())
	configureTestSudoOwnership(t, provider)
	provider.SudoersDir, provider.SudoersPath = dir, sudoers
	provider.LookupRecovery = func(string) error { return nil }
	rollbackRoot := filepath.Join(dir, "state", "resource-transactions")
	store, err := rollbackstore.New(rollbackstore.Options{Root: rollbackRoot})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.ConfigureRollback(store, "base/developer-admin", "sha256:artifact"); err != nil {
		t.Fatal(err)
	}
	provider.ValidateEffective = func(_ context.Context, stagedSudoers, stagedDir string) error {
		if stagedSudoers == sudoers || stagedDir == dir {
			t.Fatal("effective validation must use an isolated staged tree")
		}
		content, err := os.ReadFile(filepath.Join(stagedDir, "developer-admin"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), "developer ALL=(ALL) NOPASSWD: /usr/bin/id") {
			t.Fatalf("staged fragment = %q", content)
		}
		return errors.New("visudo rejected staged policy")
	}

	if err := provider.Apply(context.Background()); err == nil {
		t.Fatal("invalid staged sudoers policy must not activate")
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "old sudo policy\n" {
		t.Fatalf("active fragment changed after failed validation: %q, %v", content, err)
	}

	provider.ValidateEffective = func(context.Context, string, string) error { return nil }
	if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	content, err = os.ReadFile(path)
	if err != nil || string(content) != "developer ALL=(ALL) NOPASSWD: /usr/bin/id\n" {
		t.Fatalf("active fragment = %q, %v", content, err)
	}
	restartedStore, err := rollbackstore.New(rollbackstore.Options{Root: rollbackRoot})
	if err != nil {
		t.Fatal(err)
	}
	restarted := sudo.New(testSudoResource())
	restarted.SudoersDir, restarted.SudoersPath = dir, sudoers
	if err := restarted.ConfigureRollback(restartedStore, "base/developer-admin", "sha256:artifact"); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Revert(context.Background()); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(path)
	if err != nil || string(content) != "old sudo policy\n" {
		t.Fatalf("protected rollback = %q, %v", content, err)
	}
}

func configureTestSudoOwnership(t *testing.T, provider *sudo.Applicator) {
	t.Helper()
	account, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	group, err := user.LookupGroupId(account.Gid)
	if err != nil {
		t.Fatal(err)
	}
	provider.Owner, provider.Group = account.Username, group.Name
}

// OS-LIA-011: an access-risk sudo resource cannot begin enforcement until a
// declared recovery principal has passed its resource-specific preflight.
func TestSudoApplicatorPreflightRequiresRecoveryPrincipal(t *testing.T) {
	provider := sudo.New(testSudoResource())
	provider.LookupRecovery = func(string) error { return errors.New("recovery principal is unavailable") }
	if err := provider.Preflight(context.Background()); err == nil {
		t.Fatal("missing recovery principal must block sudo enforcement")
	}
}

// OS-AEC-098: sudoers fragments have an implicit root:root ownership
// contract; correct text and mode under an unprivileged owner are not safe.
func TestSudoProviderRejectsNonRootOwnedCompliantFragment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "developer-admin")
	if err := os.WriteFile(path, []byte("developer ALL=(ALL) NOPASSWD: /usr/bin/id\n"), 0o440); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		if err := os.Chown(path, 65534, 65534); err != nil {
			t.Fatal(err)
		}
	}
	applicator := sudo.New(testSudoResource())
	applicator.SudoersDir = dir
	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}

	if result := provider.Check(context.Background()); result.Status != contract.Drifted {
		t.Fatalf("unprivileged-owned sudo fragment Check = %+v, want drifted", result)
	}
}
