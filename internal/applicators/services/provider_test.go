package services_test

import (
	"errors"
	"fmt"
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

// OS-AEC-098: enablement and activation are one public service-state
// transaction; a failed start must not leave a previously disabled service
// enabled.
func TestProviderNeutralServiceStartFailureRestoresPriorState(t *testing.T) {
	enabled, active := true, true
	runner := &activationFailureRunner{startFailures: 1}
	provider := newSystemServiceProvider(t, models.ServiceResource{
		Name: "qualification", Provider: models.ServiceProviderSystemd, Scope: models.ServiceScopeSystem,
		Service: "qualification.service", Enabled: &enabled, Active: &active,
	}, runner)

	result := provider.Apply(t.Context())
	if result.Status != executor.Failed || result.Err == nil {
		t.Fatalf("Apply() = %+v, want failed activation", result)
	}
	if runner.enabled || runner.active || runner.failed {
		t.Fatalf("state after failed activation = enabled:%t active:%t failed:%t, want restored disabled/inactive without a failure latch", runner.enabled, runner.active, runner.failed)
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

type activationFailureRunner struct {
	enabled       bool
	active        bool
	failed        bool
	masked        bool
	startFailures int
}

func (r *activationFailureRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	if name != "systemctl" || len(args) == 0 {
		return nil, nil, fmt.Errorf("unexpected command %s %v", name, args)
	}
	switch args[0] {
	case "is-enabled":
		switch {
		case r.masked:
			return []byte("masked\n"), nil, errors.New("masked")
		case r.enabled:
			return []byte("enabled\n"), nil, nil
		default:
			return []byte("disabled\n"), nil, errors.New("disabled")
		}
	case "is-active":
		if r.active {
			return []byte("active\n"), nil, nil
		}
		if r.failed {
			return []byte("failed\n"), nil, errors.New("failed")
		}
		return []byte("inactive\n"), nil, errors.New("inactive")
	case "daemon-reload":
		return nil, nil, nil
	case "enable":
		r.enabled = true
		return nil, nil, nil
	case "disable":
		r.enabled = false
		return nil, nil, nil
	case "start":
		if r.startFailures > 0 {
			r.startFailures--
			r.failed = true
			return nil, []byte("synthetic activation failure"), errors.New("start failed")
		}
		r.active = true
		r.failed = false
		return nil, nil, nil
	case "stop":
		r.active = false
		return nil, nil, nil
	case "reset-failed":
		r.failed = false
		return nil, nil, nil
	case "mask":
		r.masked = true
		return nil, nil, nil
	case "unmask":
		r.masked = false
		return nil, nil, nil
	default:
		return nil, nil, fmt.Errorf("unexpected systemctl argv %v", args)
	}
}
