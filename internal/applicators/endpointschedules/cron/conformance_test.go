package cron_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	cronprovider "github.com/DavidHoenisch/remotr/internal/applicators/endpointschedules/cron"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

func TestApplicatorProviderContract(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider { return cronContractProvider(t, models.LifecyclePresent, true) },
		Drifted:   func(t *testing.T) contract.Provider { return cronContractProvider(t, models.LifecyclePresent, false) },
	})
	harness.RunAbsence(t, harness.AbsenceFixture{
		Absent:  func(t *testing.T) contract.Provider { return cronContractProvider(t, models.LifecycleAbsent, false) },
		Present: cronAbsentWithPresentState,
	})
	harness.RunNegativeChecks(t, harness.NegativeFixture{
		Unsupported: func(t *testing.T) contract.Provider {
			provider := newCronProvider(t, t.TempDir(), models.LifecyclePresent)
			provider.BackendAvailable = func() error { return errors.New("cron missing") }
			return adaptCron(t, provider)
		},
		ProbeFailure: func(t *testing.T) contract.Provider {
			root := t.TempDir()
			provider := newCronProvider(t, root, models.LifecyclePresent)
			blocked := filepath.Join(root, "not-a-directory")
			if err := os.WriteFile(blocked, []byte("blocked"), 0o600); err != nil {
				t.Fatal(err)
			}
			provider.StateDir = blocked
			return adaptCron(t, provider)
		},
		Validate: func(*testing.T) error {
			return (models.EndpointScheduleResource{Name: "bad", Backend: models.ScheduleBackendCron, Schedule: "99 * * * *", User: "root", Argv: []string{"/usr/bin/true"}}).Validate()
		},
	})
}

func cronContractProvider(t *testing.T, lifecycle models.Lifecycle, compliant bool) contract.Provider {
	t.Helper()
	provider := newCronProvider(t, t.TempDir(), lifecycle)
	if compliant && lifecycle != models.LifecycleAbsent {
		if err := provider.Apply(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	return adaptCron(t, provider)
}

func cronAbsentWithPresentState(t *testing.T) contract.Provider {
	t.Helper()
	root := t.TempDir()
	present := newCronProvider(t, root, models.LifecyclePresent)
	if err := present.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	return adaptCron(t, newCronProvider(t, root, models.LifecycleAbsent))
}

func newCronProvider(t *testing.T, root string, lifecycle models.Lifecycle) *cronprovider.Applicator {
	t.Helper()
	resource := models.EndpointScheduleResource{ResourceMeta: models.ResourceMeta{Lifecycle: lifecycle}, Name: "contract", Backend: models.ScheduleBackendCron}
	if lifecycle != models.LifecycleAbsent {
		resource.Schedule, resource.User, resource.Argv = "0 3 * * *", "root", []string{"/usr/bin/true"}
	}
	provider := cronprovider.New(resource)
	provider.CronDir, provider.StateDir, provider.RunDir = filepath.Join(root, "cron.d"), filepath.Join(root, "state"), filepath.Join(root, "run")
	provider.BackendAvailable = func() error { return nil }
	provider.LookupUser = func(string) (int, int, error) { return os.Getuid(), os.Getgid(), nil }
	provider.ResolveSecret = func(context.Context, string) (string, error) { return "", nil }
	return provider
}

func adaptCron(t *testing.T, provider *cronprovider.Applicator) contract.Provider {
	t.Helper()
	adapted, err := contract.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	return adapted
}
