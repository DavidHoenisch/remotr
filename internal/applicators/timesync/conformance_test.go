package timesync_test

import (
	"fmt"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/timesync"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

func TestApplicator_ConformsForEnablement(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider { return newContractProvider(t, true) },
		Drifted:   func(t *testing.T) contract.Provider { return newContractProvider(t, false) },
	})
}

func newContractProvider(t *testing.T, enabled bool) contract.Provider {
	t.Helper()
	want := true
	provider, err := contract.New(timesync.New(models.TimeSyncResource{Name: "ntp", Provider: models.TimeSyncProviderSystemdTimesyncd, Enabled: &want}, &enablementRunner{enabled: enabled}))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

type enablementRunner struct{ enabled bool }

func (r *enablementRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	if name == "systemctl" && len(args) == 4 && args[0] == "show" && args[1] == "systemd-timesyncd.service" && args[2] == "--property=LoadState" && args[3] == "--value" {
		return []byte("loaded\n"), nil, nil
	}
	if name != "timedatectl" {
		return nil, nil, fmt.Errorf("unexpected command %s", name)
	}
	if len(args) == 3 && args[0] == "show" && args[1] == "--property=NTP" && args[2] == "--value" {
		if r.enabled {
			return []byte("yes\n"), nil, nil
		}
		return []byte("no\n"), nil, nil
	}
	if len(args) == 2 && args[0] == "set-ntp" && args[1] == "true" {
		r.enabled = true
		return nil, nil, nil
	}
	return nil, nil, fmt.Errorf("unexpected timedatectl arguments %v", args)
}
