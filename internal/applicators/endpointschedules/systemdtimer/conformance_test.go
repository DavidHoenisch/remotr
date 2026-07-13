package systemdtimer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/endpointschedules/systemdtimer"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

func TestApplicatorProviderContract(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider {
			return systemdTimerContractProvider(t, models.LifecyclePresent, true)
		},
		Drifted: func(t *testing.T) contract.Provider {
			return systemdTimerContractProvider(t, models.LifecyclePresent, false)
		},
	})
	harness.RunAbsence(t, harness.AbsenceFixture{
		Absent: func(t *testing.T) contract.Provider {
			return systemdTimerContractProvider(t, models.LifecycleAbsent, false)
		},
		Present: systemdTimerAbsentWithPresentState,
	})
}

func systemdTimerContractProvider(t *testing.T, lifecycle models.Lifecycle, compliant bool) contract.Provider {
	t.Helper()
	provider := newSystemdTimerProvider(t, t.TempDir(), lifecycle, &systemdTimerRunner{})
	if compliant && lifecycle != models.LifecycleAbsent {
		if err := provider.Apply(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	return adaptSystemdTimer(t, provider)
}

func systemdTimerAbsentWithPresentState(t *testing.T) contract.Provider {
	t.Helper()
	root := t.TempDir()
	runner := &systemdTimerRunner{}
	present := newSystemdTimerProvider(t, root, models.LifecyclePresent, runner)
	if err := present.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	return adaptSystemdTimer(t, newSystemdTimerProvider(t, root, models.LifecycleAbsent, runner))
}

func newSystemdTimerProvider(t *testing.T, root string, lifecycle models.Lifecycle, runner *systemdTimerRunner) *systemdtimer.Applicator {
	t.Helper()
	persistent := true
	resource := models.EndpointScheduleResource{ResourceMeta: models.ResourceMeta{Lifecycle: lifecycle}, Name: "contract", Backend: models.ScheduleBackendSystemdTimer}
	if lifecycle != models.LifecycleAbsent {
		resource.Schedule, resource.User, resource.Argv, resource.Persistent = "daily", "root", []string{"/usr/bin/true"}, &persistent
	}
	provider := systemdtimer.New(resource, runner)
	provider.UnitDir, provider.EnvironmentDir = filepath.Join(root, "units"), filepath.Join(root, "environment")
	provider.LookupUser = func(string) (int, int, error) { return os.Getuid(), os.Getgid(), nil }
	provider.ResolveSecret = func(context.Context, string) (string, error) { return "", nil }
	provider.ValidateUnits = func(context.Context, string, string) error { return nil }
	return provider
}

func adaptSystemdTimer(t *testing.T, provider *systemdtimer.Applicator) contract.Provider {
	t.Helper()
	adapted, err := contract.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	return adapted
}
