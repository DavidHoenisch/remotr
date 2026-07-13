package mounts_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/mounts"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

func TestApplicator_ConformsForMountedPersistentState(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider { return newContractProvider(t, true) },
		Drifted:   func(t *testing.T) contract.Provider { return newContractProvider(t, false) },
	})
}

func newContractProvider(t *testing.T, mounted bool) contract.Provider {
	t.Helper()
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	mountInfo := filepath.Join(root, "mountinfo")
	runner := &mountRunner{mountInfo: mountInfo, target: target}
	if mounted {
		if err := runner.recordMount(); err != nil {
			t.Fatal(err)
		}
	} else if err := os.WriteFile(mountInfo, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	fstab := filepath.Join(root, "fstab")
	if mounted {
		if err := os.WriteFile(fstab, []byte("tmpfs "+target+" tmpfs mode=0755 0 0 # remotr:cache\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wantMounted, wantPersistent := true, true
	provider := mounts.New(models.MountResource{Name: "cache", Source: "tmpfs", Target: target, FilesystemType: "tmpfs", Options: []string{"mode=0755"}, Mounted: &wantMounted, Persistent: &wantPersistent}, runner)
	provider.FstabPath = fstab
	provider.MountInfoPath = mountInfo
	provider.FilesystemsPath = filepath.Join(root, "filesystems")
	provider.StateDir = filepath.Join(root, "state")
	if err := os.WriteFile(provider.FilesystemsPath, []byte("nodev\ttmpfs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	value, err := contract.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

type mountRunner struct{ mountInfo, target string }

func (r *mountRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	if name != "mount" || len(args) != 6 || args[0] != "-t" || args[1] != "tmpfs" || args[2] != "-o" || args[3] != "mode=0755" || args[4] != "tmpfs" || args[5] != r.target {
		return nil, nil, fmt.Errorf("unexpected command %s %v", name, args)
	}
	return nil, nil, r.recordMount()
}

func (r *mountRunner) recordMount() error {
	return os.WriteFile(r.mountInfo, []byte("36 25 0:28 / "+r.target+" rw - tmpfs tmpfs mode=0755\n"), 0o644)
}
