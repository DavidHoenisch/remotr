package accountlimits_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/accountlimits"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

func TestApplicatorProviderContract(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider {
			return accountLimitContractProvider(t, models.LifecyclePresent, true)
		},
		Drifted: func(t *testing.T) contract.Provider {
			return accountLimitContractProvider(t, models.LifecyclePresent, false)
		},
	})
	harness.RunAbsence(t, harness.AbsenceFixture{
		Absent: func(t *testing.T) contract.Provider {
			return accountLimitContractProvider(t, models.LifecycleAbsent, false)
		},
		Present: func(t *testing.T) contract.Provider {
			return accountLimitContractProvider(t, models.LifecycleAbsent, true)
		},
	})
}

func accountLimitContractProvider(t *testing.T, lifecycle models.Lifecycle, installed bool) contract.Provider {
	t.Helper()
	resource := models.AccountLimitResource{ResourceMeta: models.ResourceMeta{Lifecycle: lifecycle}, Name: "build"}
	if lifecycle == models.LifecyclePresent {
		resource.Entries = []models.AccountLimitEntry{{Domain: "@build", Type: models.AccountLimitSoft, Item: "nofile", Value: "65536"}}
	}
	provider := accountlimits.New(resource)
	provider.LimitsDir = t.TempDir()
	if installed {
		content := "stale\n"
		if lifecycle == models.LifecyclePresent {
			content = "@build soft nofile 65536\n"
		}
		if err := os.WriteFile(filepath.Join(provider.LimitsDir, "90-remotr-build.conf"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	adapted, err := contract.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	return adapted
}
