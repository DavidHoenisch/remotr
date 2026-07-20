package apt_test

import (
	"context"
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

// OS-PRM-006: changing APT hold policy must not reinstall an otherwise
// compliant package, and the updated hold state must be idempotent.
func TestApplicator_ConformsForHoldAndUnhold(t *testing.T) {
	for _, desiredHold := range []bool{true, false} {
		name := "unhold"
		if desiredHold {
			name = "hold"
		}
		t.Run(name, func(t *testing.T) {
			runner := &contractRunner{installed: true, held: !desiredHold}
			provider, err := contract.New(apt.New(models.Package{
				Name: "contract-package", Present: true, Hold: &desiredHold,
			}, runner))
			if err != nil {
				t.Fatal(err)
			}

			if check := provider.Check(context.Background()); check.Status != contract.Drifted {
				t.Fatalf("initial Check() = %+v, want drift", check)
			}
			if result := provider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
				t.Fatalf("Apply() = %+v, want changed", result)
			}
			if check := provider.Check(context.Background()); check.Status != contract.Compliant {
				t.Fatalf("second Check() = %+v, want compliant", check)
			}
			if result := provider.Apply(context.Background()); result.Status != contract.NoChange || result.Err != nil {
				t.Fatalf("second Apply() = %+v, want no-change", result)
			}
			if runner.packageMutations != 0 {
				t.Fatalf("hold-only convergence ran %d package transaction(s)", runner.packageMutations)
			}
		})
	}
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
	installed        bool
	held             bool
	packageMutations int
}

func (r *contractRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	switch {
	case name == "dpkg" && len(args) == 2 && args[0] == "-s":
		if r.installed {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("package %q is not installed", args[1])
	case name == "apt-mark" && len(args) == 2:
		switch args[0] {
		case "showhold":
			if r.held {
				return []byte(args[1] + "\n"), nil, nil
			}
			return nil, nil, nil
		case "hold":
			r.held = true
			return nil, nil, nil
		case "unhold":
			r.held = false
			return nil, nil, nil
		}
	case name == "apt-get" && len(args) == 3 && args[1] == "-y":
		r.packageMutations++
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
