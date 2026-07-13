package swaps_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/swaps"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-MSM-006: absent swap files are zero-filled before formatting and
// activation; truncation is deliberately not used because it creates holes.
func TestApplicator_CreatesProtectedNonSparseSwapFile(t *testing.T) {
	active, persistent := true, true
	dir := t.TempDir()
	path := filepath.Join(dir, "swap")
	swapsFile := filepath.Join(dir, "swaps")
	if err := os.WriteFile(swapsFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &swapRunner{path: path}
	applicator := swaps.New(models.SwapResource{Name: "page", Path: path, Type: "file", SizeBytes: 2 << 20, Priority: 5, Active: &active, Persistent: &persistent}, runner)
	applicator.SwapsPath = swapsFile
	applicator.FstabPath = filepath.Join(dir, "fstab")
	if err := applicator.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("calls=%#v", runner.calls)
	}
	got, err := os.ReadFile(applicator.FstabPath)
	if err != nil || string(got) != path+" none swap pri=5 0 0 # remotr:page\n" {
		t.Fatalf("fstab=%q err=%v", got, err)
	}
}

type swapRunner struct {
	path  string
	calls [][]string
}

func (r *swapRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	if name == "dd" {
		return nil, nil, os.WriteFile(r.path, make([]byte, 2<<20), 0o600)
	}
	return nil, nil, nil
}

func TestSwapRemovalRequiresExplicitControl(t *testing.T) {
	active := false
	if err := (models.SwapResource{Name: "page", Path: "/swap", Type: "file", SizeBytes: 1, Active: &active}).Validate(); err == nil {
		t.Fatal("unsafe removal accepted")
	}
}
