package files_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/files"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

const ackAddress = "password-policy/pass-max-days"

func armedCount(t *testing.T, store *rollbackstore.Store) int {
	t.Helper()
	records, err := store.Records(context.Background(), ackAddress)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, record := range records {
		if record.Armed {
			n++
		}
	}
	return n
}

func newAckProvider(t *testing.T, store *rollbackstore.Store, path, digest string) *files.Applicator {
	t.Helper()
	provider := files.New(models.File{Name: "pass-max-days", Path: path, Content: "PASS_MAX_DAYS 90\n"})
	if err := provider.ConfigureRollback(store, ackAddress, digest); err != nil {
		t.Fatal(err)
	}
	return provider
}

// OS-AEC-122: a successful change that a later Check proves compliant is
// acknowledged, so the next rollback preflight for the resource succeeds.
func TestFileRollbackAcknowledgedAfterCompliantCheck(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "login.defs")
	if err := os.WriteFile(path, []byte("PASS_MAX_DAYS 99999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := rollbackstore.New(rollbackstore.Options{Root: filepath.Join(dir, "resource-transactions")})
	if err != nil {
		t.Fatal(err)
	}
	provider := newAckProvider(t, store, path, "sha256:artifact")
	if err := provider.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if armedCount(t, store) != 1 {
		t.Fatal("apply did not arm a rollback record")
	}
	if check := executor.Check(ctx, provider); check.Status != executor.Compliant {
		t.Fatalf("Check() = %+v", check)
	}
	var acknowledger executor.ConvergedRollbackAcknowledger = provider
	if err := acknowledger.AcknowledgeConvergedRollback(ctx); err != nil {
		t.Fatal(err)
	}
	if n := armedCount(t, store); n != 0 {
		t.Fatalf("armed records after compliant acknowledgement = %d", n)
	}
	if err := provider.PreflightRollback(ctx); err != nil {
		t.Fatalf("PreflightRollback() after acknowledgement = %v", err)
	}
}

// OS-AEC-123: a record armed by an earlier agent process (different store
// instance and artifact digest) is acknowledged once the file is compliant,
// without changing the managed file.
func TestFileRollbackAcknowledgesRecordFromEarlierProcess(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "login.defs")
	if err := os.WriteFile(path, []byte("PASS_MAX_DAYS 99999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "resource-transactions")
	earlier, err := rollbackstore.New(rollbackstore.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := newAckProvider(t, earlier, path, "sha256:old-artifact").Apply(ctx); err != nil {
		t.Fatal(err)
	}

	restarted, err := rollbackstore.New(rollbackstore.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	provider := newAckProvider(t, restarted, path, "sha256:new-artifact")
	if err := provider.PreflightRollback(ctx); err == nil {
		t.Fatal("stale armed record did not block rollback preflight before acknowledgement")
	}
	if err := provider.AcknowledgeConvergedRollback(ctx); err != nil {
		t.Fatal(err)
	}
	if n := armedCount(t, restarted); n != 0 {
		t.Fatalf("armed records after acknowledgement = %d", n)
	}
	if err := provider.PreflightRollback(ctx); err != nil {
		t.Fatalf("PreflightRollback() after acknowledgement = %v", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "PASS_MAX_DAYS 90\n" {
		t.Fatalf("acknowledgement changed managed file: %q, %v", data, err)
	}
}

// OS-AEC-124: a non-compliant resource keeps its armed record and stays
// blocked pending recovery.
func TestFileRollbackNotAcknowledgedWhenNotCompliant(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "login.defs")
	if err := os.WriteFile(path, []byte("PASS_MAX_DAYS 99999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "resource-transactions")
	earlier, err := rollbackstore.New(rollbackstore.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := newAckProvider(t, earlier, path, "sha256:artifact").Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("PASS_MAX_DAYS 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	restarted, err := rollbackstore.New(rollbackstore.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	provider := newAckProvider(t, restarted, path, "sha256:artifact")
	if err := provider.AcknowledgeConvergedRollback(ctx); err != nil {
		t.Fatal(err)
	}
	if n := armedCount(t, restarted); n != 1 {
		t.Fatalf("armed records for non-compliant resource = %d, want 1", n)
	}
	if err := provider.PreflightRollback(ctx); err == nil {
		t.Fatal("rollback preflight succeeded while an interrupted change remains armed")
	}
}
