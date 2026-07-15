package apparmor_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/apparmor"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

func TestApplicatorProviderContract(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider { return appArmorContractProvider(t, true) },
		Drifted:   func(t *testing.T) contract.Provider { return appArmorContractProvider(t, false) },
	})
}

func appArmorContractProvider(t *testing.T, compliant bool) contract.Provider {
	t.Helper()
	dir := t.TempDir()
	content := "profile service { /managed r, }\n"
	if compliant {
		if err := os.WriteFile(filepath.Join(dir, "remotr-service"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	current := models.AppArmorEnforce
	runner := &modeRunner{mode: &current, desired: models.AppArmorEnforce}
	provider := apparmor.New(models.AppArmorProfileResource{Name: "service", Profile: "service", Content: content, Mode: models.AppArmorEnforce}, runner)
	provider.ProfilesDir = dir
	provider.DisableDir = filepath.Join(dir, "disable")
	provider.ObserveMode = func(context.Context, string) (models.AppArmorMode, error) { return current, nil }
	adapted, err := contract.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	return adapted
}
