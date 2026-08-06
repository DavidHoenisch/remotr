//go:build vmsafety

package bootstrap_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/bootstrap"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestBootstrapProviderUbuntu2604VM(t *testing.T) {
	testBootstrapProviderCoreDeliveryVM(t)
}

func TestBootstrapProviderUbuntu2404VM(t *testing.T) {
	testBootstrapProviderCoreDeliveryVM(t)
}

func TestBootstrapProviderPopOS2404VM(t *testing.T) {
	testBootstrapProviderCoreDeliveryVM(t)
}

func testBootstrapProviderCoreDeliveryVM(t *testing.T) {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "bootstrap-complete")
	provider := bootstrap.New(models.BootstrapResource{
		Name: "qualification", When: models.BootstrapWhen{PathMissing: marker},
		Steps: []models.BootstrapStep{{Exec: []string{"/usr/bin/touch", marker}}},
	}, nil)
	if _, compliant := provider.State(t.Context()); compliant {
		t.Fatal("missing completion marker reported compliant")
	}
	if err := provider.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, compliant := provider.State(t.Context()); !compliant {
		t.Fatal("bootstrap did not converge")
	}
	if err := provider.Apply(t.Context()); !errors.Is(err, appErr.ErrStateAlreadyMet) {
		t.Fatalf("second Apply() = %v", err)
	}
}
