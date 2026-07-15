package journald_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/journald"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

func TestApplicatorProviderContract(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider { return presentContractProvider(t, true) },
		Drifted:   func(t *testing.T) contract.Provider { return presentContractProvider(t, false) },
	})
	harness.RunAbsence(t, harness.AbsenceFixture{
		Absent:  func(t *testing.T) contract.Provider { return absentContractProvider(t, false) },
		Present: func(t *testing.T) contract.Provider { return absentContractProvider(t, true) },
	})
}

func presentContractProvider(t *testing.T, compliant bool) contract.Provider {
	t.Helper()
	root := t.TempDir()
	configDir := filepath.Join(root, "journald.conf.d")
	mainConfig := filepath.Join(root, "journald.conf")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainConfig, []byte("[Journal]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if compliant {
		if err := os.WriteFile(filepath.Join(configDir, "90-remotr-contract.conf"), []byte("[Journal]\nStorage=persistent\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	provider := journald.New(models.JournaldResource{Name: "contract", Storage: models.JournaldStoragePersistent}, nil)
	provider.ConfigDir, provider.MainConfig = configDir, mainConfig
	provider.ValidateEffective = func(context.Context, string, string, string) error { return nil }
	adapted, err := contract.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	return adapted
}

func absentContractProvider(t *testing.T, present bool) contract.Provider {
	t.Helper()
	root := t.TempDir()
	configDir := filepath.Join(root, "journald.conf.d")
	mainConfig := filepath.Join(root, "journald.conf")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainConfig, []byte("[Journal]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if present {
		if err := os.WriteFile(filepath.Join(configDir, "90-remotr-contract.conf"), []byte("[Journal]\nStorage=persistent\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	provider := journald.New(models.JournaldResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: "contract",
	}, nil)
	provider.ConfigDir, provider.MainConfig = configDir, mainConfig
	provider.ValidateEffective = func(context.Context, string, string, string) error { return nil }
	adapted, err := contract.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	return adapted
}
