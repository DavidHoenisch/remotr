package kernelmodules_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/kernelmodules"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

func TestApplicator_ConformsForLoadedPersistentModule(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider { return newContractProvider(t, true) },
		Drifted:   func(t *testing.T) contract.Provider { return newContractProvider(t, false) },
	})
}

func newContractProvider(t *testing.T, loaded bool) contract.Provider {
	t.Helper()
	root := t.TempDir()
	modules := filepath.Join(root, "proc", "modules")
	if err := os.MkdirAll(filepath.Dir(modules), 0o755); err != nil {
		t.Fatal(err)
	}
	if loaded {
		if err := os.WriteFile(modules, []byte("loop 1 0 - Live 0x0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	} else if err := os.WriteFile(modules, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	wantLoaded, wantPersistent := true, true
	applicator := kernelmodules.New(models.KernelModuleResource{
		Name:       "loop",
		Module:     "loop",
		Loaded:     &wantLoaded,
		Persistent: &wantPersistent,
		Parameters: map[string]string{"max_loop": "64"},
	}, &contractRunner{modules: modules, parameter: filepath.Join(root, "sys", "module", "loop", "parameters", "max_loop")})
	applicator.ProcModules = modules
	applicator.SysModuleRoot = filepath.Join(root, "sys", "module")
	applicator.ModulesLoadDir = filepath.Join(root, "modules-load.d")
	applicator.ModprobeDir = filepath.Join(root, "modprobe.d")
	applicator.HasModprobe = func() bool { return true }

	if loaded {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(applicator.SysModuleRoot, "loop", "parameters", "max_loop")), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(applicator.SysModuleRoot, "loop", "parameters", "max_loop"), []byte("64\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(applicator.ModulesLoadDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(applicator.ModulesLoadDir, "99-remotr-loop.conf"), []byte("loop\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(applicator.ModprobeDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(applicator.ModprobeDir, "99-remotr-loop.conf"), []byte("options loop max_loop=64\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	provider, err := contract.New(applicator)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

type contractRunner struct {
	modules   string
	parameter string
}

func (r *contractRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	if name != "modprobe" || len(args) != 2 || args[0] != "loop" || args[1] != "max_loop=64" {
		return nil, nil, fmt.Errorf("unexpected command %s %v", name, args)
	}
	if err := os.WriteFile(r.modules, []byte("loop 1 0 - Live 0x0\n"), 0o644); err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(filepath.Dir(r.parameter), 0o755); err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(r.parameter, []byte("64\n"), 0o644); err != nil {
		return nil, nil, err
	}
	return nil, nil, nil
}
