package accountlimits_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/accountlimits"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

// OS-AEC-100 / OS-LIA-012: a drifted named fragment must not replace active
// state when any sibling in the complete pam_limits configuration is invalid.
func TestApplicatorRejectsInvalidFullConfigurationBeforeMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "90-remotr-build.conf")
	previous := []byte("@build soft nofile 1024\n")
	if err := os.WriteFile(path, previous, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "80-unmanaged.conf"), []byte("invalid full configuration\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	applicator := accountlimits.New(models.AccountLimitResource{
		Name: "build", Entries: []models.AccountLimitEntry{
			{Domain: "@build", Type: models.AccountLimitSoft, Item: "nofile", Value: "4096"},
		},
	})
	applicator.LimitsDir = dir

	result := applicator.ApplyResult(context.Background())
	if result.Status != executor.Failed || result.Err == nil {
		t.Fatalf("ApplyResult() = %+v, want failed full-configuration validation", result)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, previous) {
		t.Fatalf("managed fragment after failed validation = %q, %v", got, err)
	}
}

// OS-LIA-012: changing a named limits fragment reports logout-required but
// never terminates an active session as an incidental action.
func TestApplicatorConvergesNamedLimitsAndReportsLogoutRequired(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "90-remotr-build.conf")
	if err := os.WriteFile(path, []byte("@build soft nofile 1024\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	applicator := accountlimits.New(models.AccountLimitResource{
		Name: "build", Entries: []models.AccountLimitEntry{
			{Domain: "@build", Type: models.AccountLimitSoft, Item: "nofile", Value: "65536"},
			{Domain: "@build", Type: models.AccountLimitHard, Item: "nofile", Value: "65536"},
		},
	})
	applicator.LimitsDir = dir
	rollbackRoot := filepath.Join(dir, "state", "resource-transactions")
	store, err := rollbackstore.New(rollbackstore.Options{Root: rollbackRoot})
	if err != nil {
		t.Fatal(err)
	}
	if err := applicator.ConfigureRollback(store, "base/build-limits", "sha256:artifact"); err != nil {
		t.Fatal(err)
	}
	result := applicator.ApplyResult(context.Background())
	want := []executor.ActivationSignal{{Kind: executor.ActivationLogoutRequired}}
	if result.Status != executor.Changed || !slices.Equal(result.Activation, want) {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if result.RollbackClass != executor.RollbackTransactional {
		t.Fatalf("rollback class = %q", result.RollbackClass)
	}
	if check := applicator.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("second Check() = %+v", check)
	}
	if result := applicator.ApplyResult(context.Background()); result.Status != executor.NoChange || len(result.Activation) != 0 {
		t.Fatalf("idempotent ApplyResult() = %+v", result)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "@build soft nofile 65536\n@build hard nofile 65536\n" {
		t.Fatalf("limits fragment = %q err=%v", got, err)
	}
	restartedStore, err := rollbackstore.New(rollbackstore.Options{Root: rollbackRoot})
	if err != nil {
		t.Fatal(err)
	}
	restarted := accountlimits.New(applicator.Resource)
	restarted.LimitsDir = dir
	if err := restarted.ConfigureRollback(restartedStore, "base/build-limits", "sha256:artifact"); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Revert(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "@build soft nofile 1024\n" {
		t.Fatalf("restored limits fragment = %q, %v", got, err)
	}
}
