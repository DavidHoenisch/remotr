package services_test

import (
	"errors"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
)

// OS-AEC-098: the provider-neutral service seam must distinguish a failed
// systemd state probe from the declared false state rather than silently
// accepting an unobservable service as compliant.
func TestProviderNeutralServiceRejectsProbeFailureAsCompliance(t *testing.T) {
	inactive := false
	provider := newSystemServiceProvider(t, models.ServiceResource{
		Name: "qualification", Provider: models.ServiceProviderSystemd, Scope: models.ServiceScopeSystem,
		Service: "qualification.service", Active: &inactive,
	}, probeFailureRunner{})

	check := provider.Check(t.Context())
	if check.Status != executor.CheckFailed || check.Err == nil {
		t.Fatalf("Check() = %+v, want check_failed with the native probe error", check)
	}
}

func newSystemServiceProvider(t *testing.T, service models.ServiceResource, runner executil.Runner) contract.Provider {
	t.Helper()
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	resources, err := registry.Resources(&models.Configuration{Services: []models.ServiceResource{service}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 {
		t.Fatalf("service resources = %d, want 1", len(resources))
	}
	if err := resources[0].Validate(); err != nil {
		t.Fatal(err)
	}
	handler, err := resources[0].NewProvider(resourceregistry.FactoryContext{Facts: facts.Facts{Init: facts.InitSystemd}, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := contract.New(handler)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

type probeFailureRunner struct{}

func (probeFailureRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	return nil, []byte("native state unavailable"), errors.New("systemctl probe failed")
}
