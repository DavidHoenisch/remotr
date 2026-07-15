package trustanchors_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/trustanchors"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	"github.com/DavidHoenisch/remotr/internal/types"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

func TestApplicatorProviderContract(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider {
			return trustAnchorContractProvider(t, models.LifecyclePresent, true)
		},
		Drifted: func(t *testing.T) contract.Provider {
			return trustAnchorContractProvider(t, models.LifecyclePresent, false)
		},
	})
	harness.RunAbsence(t, harness.AbsenceFixture{
		Absent: func(t *testing.T) contract.Provider {
			return trustAnchorContractProvider(t, models.LifecycleAbsent, false)
		},
		Present: func(t *testing.T) contract.Provider {
			return trustAnchorContractProvider(t, models.LifecycleAbsent, true)
		},
	})
}

func trustAnchorContractProvider(t *testing.T, lifecycle models.Lifecycle, installed bool) contract.Provider {
	t.Helper()
	material, fingerprint := testAnchor(t, "Contract Root")
	resource := models.TrustAnchorResource{ResourceMeta: models.ResourceMeta{Lifecycle: lifecycle}, Name: "contract"}
	if lifecycle == models.LifecyclePresent {
		resource.AnchorRef = "remotr:trust-anchors/contract@active"
		resource.Fingerprint = fingerprint
	}
	provider, err := trustanchors.New(resource, types.Debian)
	if err != nil {
		t.Fatal(err)
	}
	provider.AnchorsDir = t.TempDir()
	if installed {
		if err := os.WriteFile(filepath.Join(provider.AnchorsDir, "remotr-contract.crt"), material, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	provider.Resolve = func(context.Context, string) ([]byte, error) { return append([]byte(nil), material...), nil }
	adapted, err := contract.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	return adapted
}
