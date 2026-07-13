package systemduser

import (
	"fmt"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

func TestApplicator_ConformsForActiveUserService(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider { return newUserContractProvider(t, true) },
		Drifted:   func(t *testing.T) contract.Provider { return newUserContractProvider(t, false) },
	})
}

func newUserContractProvider(t *testing.T, active bool) contract.Provider {
	t.Helper()
	wantActive := true
	provider := New(models.SystemdUserResource{
		Name: "contract-user-service", Unit: "contract-user.service", Users: "interactive", Active: &wantActive,
	}, &userContractRunner{active: active})
	provider.ListUsers = func() ([]InteractiveUser, error) { return []InteractiveUser{{Username: "alice", UID: 1000}}, nil }
	adapted, err := contract.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	return adapted
}

type userContractRunner struct{ active bool }

var _ executil.Runner = (*userContractRunner)(nil)

func (r *userContractRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	if name != "sudo" || len(args) < 7 || args[4] != "systemctl" || args[5] != "--user" {
		return nil, nil, fmt.Errorf("unexpected command %s %v", name, args)
	}
	switch args[6] {
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
	default:
		return nil, nil, fmt.Errorf("unexpected systemctl --user argv %v", args)
	}
}
