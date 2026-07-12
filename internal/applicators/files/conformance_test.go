package files_test

import (
	"os"
	"path/filepath"
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
