package logrotate_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/logrotate"
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
	fragmentsDir := filepath.Join(root, "logrotate.d")
	mainConfig := filepath.Join(root, "logrotate.conf")
	if err := os.MkdirAll(fragmentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainConfig, []byte("include "+fragmentsDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if compliant {
		if err := os.WriteFile(filepath.Join(fragmentsDir, "remotr-contract"), []byte("/var/log/contract/*.log {\n  daily\n  rotate 7\n}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	retention := 7
	provider := logrotate.New(models.LogrotateResource{
		Name: "contract", Paths: []string{"/var/log/contract/*.log"}, Cadence: models.LogrotateDaily, Retention: &retention,
	}, nil)
	provider.FragmentsDir, provider.MainConfig = fragmentsDir, mainConfig
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
	fragmentsDir := filepath.Join(root, "logrotate.d")
	mainConfig := filepath.Join(root, "logrotate.conf")
	if err := os.MkdirAll(fragmentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainConfig, []byte("include "+fragmentsDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if present {
		if err := os.WriteFile(filepath.Join(fragmentsDir, "remotr-contract"), []byte("managed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	provider := logrotate.New(models.LogrotateResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: "contract",
	}, nil)
	provider.FragmentsDir, provider.MainConfig = fragmentsDir, mainConfig
	provider.ValidateEffective = func(context.Context, string, string, string) error { return nil }
	adapted, err := contract.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	return adapted
}
