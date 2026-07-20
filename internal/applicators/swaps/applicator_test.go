package swaps_test

import (
	"context"
	"errors"
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

// OS-MSM-006 / OS-AEC-098: swap-file activation is bound to the declared
// regular file and must not follow a symbolic link to another object.
func TestProviderRejectsSymlinkSwapFileBeforeActivation(t *testing.T) {
	active := true
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "swap-link")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	swapsPath := filepath.Join(dir, "swaps")
	if err := os.WriteFile(swapsPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{}
	applicator := swaps.New(models.SwapResource{
		Name: "file-identity", Path: path, Type: "file", SizeBytes: 4096, Active: &active,
	}, runner)
	applicator.SwapsPath = swapsPath
	applicator.FstabPath = filepath.Join(dir, "fstab")
	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}

	result := provider.Apply(context.Background())
	if result.Status != contract.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "symbolic link") {
		t.Fatalf("symlink swap-file Apply = %+v, want identity failure", result)
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("native calls for symlink swap file = %#v, want none", runner.Calls)
	}
}

// OS-MSM-006 / OS-AEC-098: creation must not begin when the requested
// zero-filled swap file exceeds the target filesystem's available capacity.
func TestProviderBlocksSwapFileCreationWhenCapacityIsInsufficient(t *testing.T) {
	active := true
	dir := t.TempDir()
	path := filepath.Join(dir, "swapfile")
	swapsPath := filepath.Join(dir, "swaps")
	if err := os.WriteFile(swapsPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{}
	applicator := swaps.New(models.SwapResource{
		Name: "capacity", Path: path, Type: "file", SizeBytes: 1 << 62,
		Active: &active,
	}, runner)
	applicator.SwapsPath = swapsPath
	applicator.FstabPath = filepath.Join(dir, "fstab")
	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}

	result := provider.Apply(context.Background())
	if result.Status != contract.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "capacity") {
		t.Fatalf("insufficient-capacity creation Apply = %+v, want protected failure", result)
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("native calls after capacity failure = %#v, want none", runner.Calls)
	}
}

// OS-MSM-006 / OS-AEC-098: failure after Remotr creates a swap file removes
// only that new artifact rather than leaving partial storage state behind.
func TestProviderRemovesNewSwapFileWhenFormattingFails(t *testing.T) {
	active := true
	dir := t.TempDir()
	path := filepath.Join(dir, "swapfile")
	swapsPath := filepath.Join(dir, "swaps")
	if err := os.WriteFile(swapsPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	runner := swapRunnerFunc(func(name string, args ...string) ([]byte, []byte, error) {
		switch name {
		case "dd":
			return nil, nil, os.WriteFile(path, make([]byte, 1<<20), 0o600)
		case "mkswap":
			return nil, []byte("format rejected"), errors.New("exit 1")
		default:
			return nil, nil, nil
		}
	})
	applicator := swaps.New(models.SwapResource{
		Name: "format-recovery", Path: path, Type: "file", SizeBytes: 1 << 20,
		Active: &active,
	}, runner)
	applicator.SwapsPath = swapsPath
	applicator.FstabPath = filepath.Join(dir, "fstab")
	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}

	result := provider.Apply(context.Background())
	if result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("format failure Apply = %+v, want failed", result)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("new swap file survived format failure: %v", err)
	}
}

type swapRunnerFunc func(name string, args ...string) ([]byte, []byte, error)

func (run swapRunnerFunc) Run(name string, args ...string) ([]byte, []byte, error) {
	return run(name, args...)
}

// OS-MSM-006 / OS-AEC-098: a combined active/persistent resource must stage
// its boot declaration before creating or activating live swap state.
func TestProviderDoesNotActivateWhenFstabPersistenceFails(t *testing.T) {
	active, persistent := true, true
	dir := t.TempDir()
	path := filepath.Join(dir, "swapfile")
	swapsPath := filepath.Join(dir, "swaps")
	if err := os.WriteFile(swapsPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var calls []string
	runner := swapRunnerFunc(func(name string, args ...string) ([]byte, []byte, error) {
		calls = append(calls, name)
		if name == "dd" {
			return nil, nil, os.WriteFile(path, make([]byte, 1<<20), 0o600)
		}
		return nil, nil, nil
	})
	applicator := swaps.New(models.SwapResource{
		Name: "transactional", Path: path, Type: "file", SizeBytes: 1 << 20,
		Active: &active, Persistent: &persistent,
	}, runner)
	applicator.SwapsPath = swapsPath
	applicator.FstabPath = filepath.Join("/proc", "remotr-swap-test", "fstab")
	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}

	result := provider.Apply(context.Background())
	if result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("persistence failure Apply = %+v, want failed", result)
	}
	if len(calls) != 0 {
		t.Fatalf("native calls after persistence failure = %v, want none", calls)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("swap file created before persistence failed: %v", err)
	}
}

// OS-MSM-006 / OS-AEC-098: native activation failure restores the exact
// previous fstab and removes the file created by this Apply.
func TestProviderRestoresFstabAndFileWhenActivationFails(t *testing.T) {
	active, persistent := true, true
	dir := t.TempDir()
	path := filepath.Join(dir, "swapfile")
	swapsPath := filepath.Join(dir, "swaps")
	if err := os.WriteFile(swapsPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	fstab := filepath.Join(dir, "fstab")
	original := []byte("# preserve this exact boot configuration\n")
	if err := os.WriteFile(fstab, original, 0o640); err != nil {
		t.Fatal(err)
	}
	runner := swapRunnerFunc(func(name string, args ...string) ([]byte, []byte, error) {
		switch name {
		case "dd":
			return nil, nil, os.WriteFile(path, make([]byte, 1<<20), 0o600)
		case "swapon":
			return nil, []byte("activation rejected"), errors.New("exit 255")
		default:
			return nil, nil, nil
		}
	})
	applicator := swaps.New(models.SwapResource{
		Name: "activation-recovery", Path: path, Type: "file", SizeBytes: 1 << 20,
		Active: &active, Persistent: &persistent,
	}, runner)
	applicator.SwapsPath = swapsPath
	applicator.FstabPath = fstab
	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}

	result := provider.Apply(context.Background())
	if result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("activation failure Apply = %+v, want failed", result)
	}
	contents, err := os.ReadFile(fstab)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != string(original) {
		t.Fatalf("fstab after activation failure = %q, want %q", contents, original)
	}
	info, err := os.Stat(fstab)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("fstab mode after activation failure = %04o, want 0640", info.Mode().Perm())
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("new swap file survived activation failure: %v", err)
	}
}
