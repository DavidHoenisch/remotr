//go:build vmsafety

package command_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/command"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestCommandProviderUbuntu2604VM(t *testing.T) {
	testCommandProviderCoreDeliveryVM(t)
}

func TestCommandProviderPopOS2404VM(t *testing.T) {
	testCommandProviderCoreDeliveryVM(t)
}

func testCommandProviderCoreDeliveryVM(t *testing.T) {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "command-applied")
	provider := command.New(models.CommandResource{
		Name: "qualification", Check: []string{"/usr/bin/test", "-e", marker},
		Apply: []string{"/usr/bin/touch", marker}, Revert: []string{"/usr/bin/rm", "-f", marker},
	}, nil)
	if _, compliant := provider.State(t.Context()); compliant {
		t.Fatal("missing marker reported compliant")
	}
	if err := provider.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, compliant := provider.State(t.Context()); !compliant {
		t.Fatal("applied command did not converge")
	}
	if err := provider.Apply(t.Context()); !errors.Is(err, appErr.ErrStateAlreadyMet) {
		t.Fatalf("second Apply() = %v", err)
	}
	if err := provider.Revert(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, compliant := provider.State(t.Context()); compliant {
		t.Fatal("Revert() did not restore drift")
	}
}
