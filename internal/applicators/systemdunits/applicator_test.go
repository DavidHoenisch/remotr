package systemdunits_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/systemdunits"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-SRM-003: absence removes only the named drop-in and requests one
// controlled daemon reload after the successful filesystem change.
func TestApplicatorAbsentRemovesOnlyNamedDropIn(t *testing.T) {
	unitDir := t.TempDir()
	dropInDir := filepath.Join(unitDir, "telemetry.service.d")
	if err := os.MkdirAll(dropInDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dropInDir, "20-remotr.conf")
	sibling := filepath.Join(dropInDir, "90-local.conf")
	for path, content := range map[string]string{target: "managed\n", sibling: "local\n"} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	provider := systemdunits.New(models.SystemdUnitResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: "telemetry-limits",
		Unit: "telemetry.service", DropIn: "20-remotr.conf",
	}, nil)
	provider.UnitDir = unitDir

	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Changed || len(result.Activation) != 1 || result.Activation[0].Kind != executor.ActivationDaemonReload {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("named drop-in still exists: %v", err)
	}
	if got, err := os.ReadFile(sibling); err != nil || string(got) != "local\n" {
		t.Fatalf("sibling drop-in = %q, %v", got, err)
	}
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("Check() after removal = %+v", check)
	}
	if second := provider.ApplyResult(context.Background()); second.Status != executor.NoChange || len(second.Activation) != 0 {
		t.Fatalf("second ApplyResult() = %+v", second)
	}
}

func TestApplicatorValidatesAndAtomicallyConvergesOwnedUnit(t *testing.T) {
	unitDir := t.TempDir()
	runner := &validatingRunner{t: t, unitDir: unitDir}
	provider := systemdunits.New(models.SystemdUnitResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Notifications: []models.Notification{{Type: models.NotificationTryRestart, Target: "telemetry.service"}}}, Name: "telemetry-unit", Unit: "telemetry.service",
		Content: "[Service]\nExecStart=/usr/bin/true\n", Mode: []int{0o640}, Owner: "agent", Group: "agent",
	}, runner)
	provider.UnitDir = unitDir
	provider.LookupOwner = func(owner, group string) (int, int, error) {
		if owner != "agent" || group != "agent" {
			t.Fatalf("ownership lookup = %q:%q", owner, group)
		}
		return os.Getuid(), os.Getgid(), nil
	}

	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Changed || len(result.Activation) != 2 || result.Activation[0].Kind != executor.ActivationDaemonReload || result.Activation[1].Kind != executor.ActivationTryRestart || result.Activation[1].Target != "telemetry.service" {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if runner.calls != 1 {
		t.Fatalf("validation calls = %d, want one", runner.calls)
	}
	path := filepath.Join(unitDir, "telemetry.service")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("unit metadata = %v, %v", info, err)
	}
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("Check() after Apply = %+v", check)
	}
}

type validatingRunner struct {
	t       *testing.T
	unitDir string
	calls   int
}

var _ executil.Runner = (*validatingRunner)(nil)

func (r *validatingRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.t.Helper()
	r.calls++
	if name != "env" || len(args) != 4 || !strings.HasPrefix(args[0], "SYSTEMD_UNIT_PATH=") || !strings.Contains(args[0], ":"+r.unitDir+":") || args[1] != "systemd-analyze" || args[2] != "verify" || args[3] != "telemetry.service" {
		r.t.Fatalf("validation argv = %s %v", name, args)
	}
	return nil, nil, nil
}

// OS-SRM-004: staged verification fails before the active unit is replaced or
// any activation signal can reach the service manager.
func TestApplicatorValidationFailurePreservesActiveUnit(t *testing.T) {
	unitDir := t.TempDir()
	activePath := filepath.Join(unitDir, "telemetry.service")
	if err := os.WriteFile(activePath, []byte("[Service]\nExecStart=/usr/bin/true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := systemdunits.New(models.SystemdUnitResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}, Name: "telemetry-unit", Unit: "telemetry.service",
		Content: "[Service]\nInvalidDirective=yes\n", Owner: "root", Group: "root", Mode: []int{0o644},
	}, nil)
	provider.UnitDir = unitDir
	provider.LookupOwner = func(string, string) (int, int, error) { return os.Getuid(), os.Getgid(), nil }
	provider.ValidateUnit = func(_ context.Context, _ string, stagedPath, unit string) error {
		if stagedPath == activePath || unit != "telemetry.service" {
			t.Fatalf("validation boundary = %q, %q", stagedPath, unit)
		}
		staged, err := os.ReadFile(stagedPath)
		if err != nil {
			return err
		}
		if string(staged) != provider.Resource.Content {
			t.Fatalf("staged content = %q", staged)
		}
		return errors.New("systemd-analyze: unknown directive InvalidDirective")
	}

	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Failed || result.Err == nil || len(result.Activation) != 0 {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if got, err := os.ReadFile(activePath); err != nil || string(got) != "[Service]\nExecStart=/usr/bin/true\n" {
		t.Fatalf("active unit = %q, %v", got, err)
	}
}
