package hostlocale_test

import (
	"fmt"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/hostlocale"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

func TestApplicator_ConformsForTimezoneOnlyState(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider { return newContractProvider(t, "Europe/Berlin") },
		Drifted:   func(t *testing.T) contract.Provider { return newContractProvider(t, "UTC") },
	})
}

func newContractProvider(t *testing.T, observedTimezone string) contract.Provider {
	t.Helper()
	timezone := "Europe/Berlin"
	provider, err := contract.New(hostlocale.New(models.HostLocaleResource{Name: "berlin", Timezone: &timezone}, &timezoneRunner{timezone: observedTimezone}))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

type timezoneRunner struct{ timezone string }

func (r *timezoneRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	if name != "timedatectl" {
		return nil, nil, fmt.Errorf("unexpected command %s", name)
	}
	if len(args) == 3 && args[0] == "show" && args[1] == "--property=Timezone" && args[2] == "--value" {
		return []byte(r.timezone + "\n"), nil, nil
	}
	if len(args) == 2 && args[0] == "set-timezone" {
		r.timezone = args[1]
		return nil, nil, nil
	}
	return nil, nil, fmt.Errorf("unexpected timedatectl arguments %v", args)
}
