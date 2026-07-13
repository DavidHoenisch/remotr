package systemdunits_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/systemdunits"
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
	unitDir := t.TempDir()
	resource := models.SystemdUnitResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}, Name: "contract", Unit: "contract.service",
		Content: "[Service]\nExecStart=/usr/bin/true\n", Mode: []int{0o644}, Owner: "test", Group: "test",
	}
	if compliant {
		if err := os.WriteFile(filepath.Join(unitDir, resource.Unit), []byte(resource.Content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	provider := systemdunits.New(resource, nil)
	provider.UnitDir = unitDir
	provider.LookupOwner = func(string, string) (int, int, error) { return os.Getuid(), os.Getgid(), nil }
	provider.ValidateUnit = func(context.Context, string, string, string) error { return nil }
	adapted, err := contract.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	return adapted
}

func absentContractProvider(t *testing.T, present bool) contract.Provider {
	t.Helper()
	unitDir := t.TempDir()
	resource := models.SystemdUnitResource{ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: "contract", Unit: "contract.service"}
	if present {
		if err := os.WriteFile(filepath.Join(unitDir, resource.Unit), []byte("managed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	provider := systemdunits.New(resource, nil)
	provider.UnitDir = unitDir
	adapted, err := contract.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	return adapted
}
