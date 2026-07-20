package pacman_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/packages/pacman"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

// OS-PRM-001 and OS-PRM-002: Pacman present and absent lifecycle converges
// through the public provider contract and remains compliant on a second Check.
func TestApplicator_ConformsForPresenceAndRemoval(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider { return newLifecycleProvider(t, true, true) },
		Drifted:   func(t *testing.T) contract.Provider { return newLifecycleProvider(t, true, false) },
	})
	harness.RunAbsence(t, harness.AbsenceFixture{
		Absent:  func(t *testing.T) contract.Provider { return newLifecycleProvider(t, false, false) },
		Present: func(t *testing.T) contract.Provider { return newLifecycleProvider(t, false, true) },
	})
}

func TestApplicator_ReportsPurgeAsUnsupportedWithoutMutation(t *testing.T) {
	runner := &lifecycleRunner{installed: true}
	provider := newContractProvider(t, models.Package{
		Name: "contract-package", ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePurged},
	}, runner)

	observation := provider.Check(context.Background())
	if observation.Status != contract.Unsupported || observation.ReasonCode != contract.ReasonProviderUnavailable {
		t.Fatalf("purged Check() = %+v, want typed unsupported", observation)
	}
	result := provider.Apply(context.Background())
	if result.Status != contract.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "purged") {
		t.Fatalf("purged Apply() = %+v, want explicit unsupported failure", result)
	}
	if !runner.installed || runner.mutations != 0 {
		t.Fatalf("unsupported purge mutated package state: installed=%t mutations=%d", runner.installed, runner.mutations)
	}
}

func newLifecycleProvider(t *testing.T, desiredPresent, installed bool) contract.Provider {
	t.Helper()
	pkg := models.Package{Name: "contract-package", Present: desiredPresent}
	if !desiredPresent {
		pkg.Lifecycle = models.LifecycleAbsent
	}
	return newContractProvider(t, pkg, &lifecycleRunner{installed: installed})
}

func newContractProvider(t *testing.T, pkg models.Package, runner *lifecycleRunner) contract.Provider {
	t.Helper()
	provider, err := contract.New(pacman.New(pkg, runner))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

type lifecycleRunner struct {
	installed bool
	mutations int
}

func (r *lifecycleRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	switch {
	case name == "pacman" && len(args) == 2 && args[0] == "-Q":
		if r.installed {
			return []byte(args[1] + " 1.0.0-1\n"), nil, nil
		}
		return nil, nil, fmt.Errorf("package %q is not installed", args[1])
	case name == "pacman" && len(args) == 3 && args[0] == "-S" && args[1] == "--noconfirm":
		r.installed = true
		r.mutations++
		return nil, nil, nil
	case name == "pacman" && len(args) == 3 && args[0] == "-R" && args[1] == "--noconfirm":
		r.installed = false
		r.mutations++
		return nil, nil, nil
	default:
		return nil, nil, fmt.Errorf("unexpected command %s %v", name, args)
	}
}
