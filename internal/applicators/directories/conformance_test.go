package directories_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/directories"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

func TestDirectoryProviderContract(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider {
			return directoryContractProvider(t, models.LifecyclePresent, true)
		},
		Drifted: func(t *testing.T) contract.Provider {
			return directoryContractProvider(t, models.LifecyclePresent, false)
		},
	})
	harness.RunAbsence(t, harness.AbsenceFixture{
		Absent: func(t *testing.T) contract.Provider {
			return directoryContractProvider(t, models.LifecycleAbsent, false)
		},
		Present: func(t *testing.T) contract.Provider {
			return directoryContractProvider(t, models.LifecycleAbsent, true)
		},
	})
}

func directoryContractProvider(t *testing.T, lifecycle models.Lifecycle, installed bool) contract.Provider {
	t.Helper()
	path := filepath.Join(t.TempDir(), "managed")
	if installed {
		if err := os.Mkdir(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	mode := []int(nil)
	if lifecycle == models.LifecyclePresent {
		mode = []int{0o750}
	}
	provider, err := contract.New(directories.New(models.DirectoryResource{
		Name: "managed", Path: path, Mode: mode,
		ResourceMeta: models.ResourceMeta{Lifecycle: lifecycle},
	}))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}
