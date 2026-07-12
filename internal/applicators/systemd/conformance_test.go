package systemd

import (
	"fmt"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

func TestApplicator_ConformsForActiveUnit(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider { return newContractProvider(t, true) },
		Drifted:   func(t *testing.T) contract.Provider { return newContractProvider(t, false) },
	})
}

func newContractProvider(t *testing.T, active bool) contract.Provider {
	t.Helper()
	wantActive := true
	provider, err := contract.New(New(models.SystemdResource{
		Name:   "contract-unit",
		Unit:   "contract-unit.service",
		Active: &wantActive,
	}, &contractRunner{active: active}))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

type contractRunner struct {
	active bool
}

var _ executil.Runner = (*contractRunner)(nil)

func (r *contractRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	if name != "systemctl" || len(args) == 0 {
		return nil, nil, fmt.Errorf("unexpected command %s %v", name, args)
	}
	switch args[0] {
	case "is-active":
		if r.active {
			return []byte("active\n"), nil, nil
		}
		return []byte("inactive\n"), nil, nil
	case "daemon-reload":
		return nil, nil, nil
	case "start":
		r.active = true
		return nil, nil, nil
	}
	return nil, nil, fmt.Errorf("unexpected systemctl argv %v", args)
}
