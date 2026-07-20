package swaps_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/swaps"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
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

func TestApplicatorApplyResultAdvertisesHonestNoRollbackContract(t *testing.T) {
	active := true
	dir := t.TempDir()
	path := filepath.Join(dir, "swap-device")
	if err := os.WriteFile(path, []byte{0}, 0o600); err != nil {
		t.Fatal(err)
	}
	swapsPath := filepath.Join(dir, "swaps")
	if err := os.WriteFile(swapsPath, []byte(path+" partition 1 1 -2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := swaps.New(models.SwapResource{Name: "page", Path: path, Type: "file", SizeBytes: 1, Active: &active}, nil)
	provider.SwapsPath = swapsPath
	result := provider.ApplyResult(context.Background())
	if result.Status != executor.NoChange || result.RebootRequired != executor.RebootNotRequired || result.RollbackClass != executor.RollbackNone {
		t.Fatalf("ApplyResult() = %+v, want valid no-change/no-rollback contract", result)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("ApplyResult validation failed: %v", err)
	}
}

// OS-MSM-006 / OS-AEC-098: a device resource must resolve to a block device;
// an ordinary file is rejected before the native swapon boundary.
func TestProviderRejectsRegularFileDeclaredAsSwapDevice(t *testing.T) {
	active := true
	dir := t.TempDir()
	path := filepath.Join(dir, "not-a-device")
	if err := os.WriteFile(path, []byte("ordinary file"), 0o600); err != nil {
		t.Fatal(err)
	}
	swapsPath := filepath.Join(dir, "swaps")
	if err := os.WriteFile(swapsPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{}
	applicator := swaps.New(models.SwapResource{
		Name: "device-identity", Path: path, Type: "device", Active: &active,
	}, runner)
	applicator.SwapsPath = swapsPath
	applicator.FstabPath = filepath.Join(dir, "fstab")
	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}

	result := provider.Apply(context.Background())
	if result.Status != contract.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "block device") {
		t.Fatalf("regular device Apply = %+v, want block-device identity failure", result)
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("native calls for regular device = %#v, want none", runner.Calls)
	}
}
