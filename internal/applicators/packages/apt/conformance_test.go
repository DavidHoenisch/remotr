package apt_test

import (
	"fmt"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/packages/apt"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

func TestApplicator_ConformsForPresenceAndRemoval(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider { return newContractProvider(t, true, true) },
		Drifted:   func(t *testing.T) contract.Provider { return newContractProvider(t, true, false) },
	})
	harness.RunAbsence(t, harness.AbsenceFixture{
		Absent:  func(t *testing.T) contract.Provider { return newContractProvider(t, false, false) },
		Present: func(t *testing.T) contract.Provider { return newContractProvider(t, false, true) },
	})
}

// OS-PRM-001: APT purge is a first-class absence lifecycle and must converge
// through the same public provider contract as ordinary removal.
func TestApplicator_ConformsForPurge(t *testing.T) {
	harness.RunAbsence(t, harness.AbsenceFixture{
		Absent:  func(t *testing.T) contract.Provider { return newPurgeContractProvider(t, false) },
		Present: func(t *testing.T) contract.Provider { return newPurgeContractProvider(t, true) },
	})
}

func newContractProvider(t *testing.T, desiredPresent, installed bool) contract.Provider {
	t.Helper()
	provider, err := contract.New(apt.New(models.Package{Name: "contract-package", Present: desiredPresent}, &contractRunner{installed: installed}))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func newPurgeContractProvider(t *testing.T, installed bool) contract.Provider {
	t.Helper()
	provider, err := contract.New(apt.New(models.Package{
		Name: "contract-package", ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePurged},
	}, &contractRunner{installed: installed}))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

type contractRunner struct {
	installed bool
}

func (r *contractRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	switch {
	case name == "dpkg" && len(args) == 2 && args[0] == "-s":
		if r.installed {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("package %q is not installed", args[1])
	case name == "apt-get" && len(args) == 3 && args[1] == "-y":
		switch args[0] {
		case "install":
			r.installed = true
			return nil, nil, nil
		case "remove", "purge":
			r.installed = false
			return nil, nil, nil
		}
	}
	return nil, nil, fmt.Errorf("unexpected command %s %v", name, args)
}
