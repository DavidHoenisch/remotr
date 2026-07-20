package files_test

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/files"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

func TestApplicator_ConformsForManagedContent(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider { return newContractProvider(t, "managed\n") },
		Drifted:   func(t *testing.T) contract.Provider { return newContractProvider(t, "unmanaged\n") },
	})
}

// OS-AEC-097: metadata-only intent still owns the file lifecycle. A missing
// empty file must be created with its real POSIX metadata, then remain stable.
func TestApplicator_ConformsForMetadataOnlyLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty-managed-file")
	handler := files.NewOwned(models.File{
		Name: "empty-managed-file", Path: path, Mode: []int{0o640},
	}, os.Getuid(), os.Getgid())
	provider, err := contract.New(handler)
	if err != nil {
		t.Fatal(err)
	}

	if observation := provider.Check(context.Background()); observation.Status != contract.Drifted {
		t.Fatalf("missing metadata-only file Check = %+v, want drifted", observation)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("metadata-only file Apply = %+v, want changed", result)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0o640 || int(stat.Uid) != os.Getuid() || int(stat.Gid) != os.Getgid() {
		t.Fatalf("created file metadata = mode:%v stat:%+v", info.Mode(), info.Sys())
	}
	if observation := provider.Check(context.Background()); observation.Status != contract.Compliant {
		t.Fatalf("second Check = %+v, want compliant", observation)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.NoChange || result.Err != nil {
		t.Fatalf("second Apply = %+v, want no-change", result)
	}
}

func newContractProvider(t *testing.T, actual string) contract.Provider {
	t.Helper()
	path := filepath.Join(t.TempDir(), "managed.conf")
	if err := os.WriteFile(path, []byte(actual), 0o600); err != nil {
		t.Fatal(err)
	}
	provider, err := contract.New(files.New(models.File{Name: "managed", Path: path, Content: "managed\n"}))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}
