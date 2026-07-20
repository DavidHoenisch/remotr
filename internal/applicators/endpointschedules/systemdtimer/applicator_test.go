package systemdtimer_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/endpointschedules/systemdtimer"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-ESM-007: paired units are staged and verified together before either
// active file changes, then daemon-reloaded once before timer enablement.
func TestApplicatorValidatesAndConvergesPairedSystemdUnits(t *testing.T) {
	persistent := true
	root := t.TempDir()
	unitDir := filepath.Join(root, "systemd")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	servicePath := filepath.Join(unitDir, "remotr-schedule-nightly.service")
	timerPath := filepath.Join(unitDir, "remotr-schedule-nightly.timer")
	if err := os.WriteFile(servicePath, []byte("old service\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(timerPath, []byte("old timer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &systemdTimerRunner{}
	provider := systemdtimer.New(models.EndpointScheduleResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}, Name: "nightly", Backend: models.ScheduleBackendSystemdTimer,
		Schedule: "*-*-* 03:00:00", User: "backup", Argv: []string{"/usr/local/bin/backup", "daily archive"}, WorkingDirectory: "/var/lib/backup", Timeout: "30m", Overlap: models.ScheduleOverlapForbid, Persistent: &persistent,
	}, runner)
	provider.UnitDir, provider.EnvironmentDir = unitDir, filepath.Join(root, "environment")
	provider.LookupUser = func(string) (int, int, error) { return os.Getuid(), os.Getgid(), nil }
	provider.ValidateUnits = func(_ context.Context, stagedService, stagedTimer string) error {
		if stagedService == servicePath || stagedTimer == timerPath {
			t.Fatal("validation used active unit paths instead of staged units")
		}
		service, err := os.ReadFile(stagedService)
		if err != nil {
			return err
		}
		timer, err := os.ReadFile(stagedTimer)
		if err != nil {
			return err
		}
		if !strings.Contains(string(service), `ExecStart=/usr/local/bin/backup "daily archive"`) || !strings.Contains(string(service), "WorkingDirectory=/var/lib/backup") || !strings.Contains(string(service), "TimeoutStartSec=30m") {
			return fmt.Errorf("staged service = %q", service)
		}
		if !strings.Contains(string(timer), "OnCalendar=*-*-* 03:00:00") || !strings.Contains(string(timer), "Persistent=true") {
			return fmt.Errorf("staged timer = %q", timer)
		}
		return nil
	}

	if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if got := runner.callStrings(); strings.Join(got, "|") != "systemctl is-enabled remotr-schedule-nightly.timer|systemctl is-active remotr-schedule-nightly.timer|systemctl daemon-reload|systemctl enable remotr-schedule-nightly.timer|systemctl start remotr-schedule-nightly.timer" {
		t.Fatalf("systemd calls = %v", got)
	}
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("Check() after Apply = %+v", check)
	}
}

// OS-ESM-007: failed staged verification preserves both active units and does
// not reload or change timer state.
func TestApplicatorVerificationFailurePreservesActivePair(t *testing.T) {
	persistent := false
	root := t.TempDir()
	unitDir := filepath.Join(root, "systemd")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	servicePath := filepath.Join(unitDir, "remotr-schedule-nightly.service")
	timerPath := filepath.Join(unitDir, "remotr-schedule-nightly.timer")
	for path, content := range map[string]string{servicePath: "old service\n", timerPath: "old timer\n"} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runner := &systemdTimerRunner{}
	provider := systemdtimer.New(models.EndpointScheduleResource{ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}, Name: "nightly", Backend: models.ScheduleBackendSystemdTimer, Schedule: "daily", User: "root", Argv: []string{"/usr/bin/true"}, Persistent: &persistent}, runner)
	provider.UnitDir, provider.EnvironmentDir = unitDir, filepath.Join(root, "environment")
	provider.LookupUser = func(string) (int, int, error) { return os.Getuid(), os.Getgid(), nil }
	provider.ValidateUnits = func(context.Context, string, string) error {
		return errors.New("systemd-analyze rejected staged timer")
	}
	if result := provider.ApplyResult(context.Background()); result.Status != executor.Failed {
		t.Fatalf("ApplyResult() = %+v, want failed", result)
	}
	for path, want := range map[string]string{servicePath: "old service\n", timerPath: "old timer\n"} {
		if got, err := os.ReadFile(path); err != nil || string(got) != want {
			t.Fatalf("active unit %s = %q, %v; want %q", path, got, err, want)
		}
	}
	if len(runner.calls) != 2 {
		t.Fatalf("runner calls after validation failure = %v, want observation only", runner.callStrings())
	}
}

// OS-AEC-098 / OS-ESM-007: a daemon-reload failure after both staged units
// have been installed restores the previously active pair and reloads that
// restored pair before reporting failure.
func TestApplicatorDaemonReloadFailureRestoresActivePair(t *testing.T) {
	persistent := false
	root := t.TempDir()
	unitDir := filepath.Join(root, "systemd")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	servicePath := filepath.Join(unitDir, "remotr-schedule-nightly.service")
	timerPath := filepath.Join(unitDir, "remotr-schedule-nightly.timer")
	previous := map[string]string{servicePath: "old service\n", timerPath: "old timer\n"}
	for path, content := range previous {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runner := &systemdTimerRunner{enabled: true, active: true, daemonReloadFailures: 1}
	provider := systemdtimer.New(models.EndpointScheduleResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}, Name: "nightly", Backend: models.ScheduleBackendSystemdTimer,
		Schedule: "daily", User: "root", Argv: []string{"/usr/bin/true"}, Persistent: &persistent,
	}, runner)
	provider.UnitDir, provider.EnvironmentDir = unitDir, filepath.Join(root, "environment")
	provider.LookupUser = func(string) (int, int, error) { return os.Getuid(), os.Getgid(), nil }
	provider.ValidateUnits = func(context.Context, string, string) error { return nil }

	if result := provider.ApplyResult(context.Background()); result.Status != executor.Failed {
		t.Fatalf("ApplyResult() = %+v, want failed", result)
	}
	for path, want := range previous {
		if got, err := os.ReadFile(path); err != nil || string(got) != want {
			t.Fatalf("active unit %s after failed reload = %q, %v; want restored %q", path, got, err, want)
		}
	}
	if got := strings.Count(strings.Join(runner.callStrings(), "|"), "systemctl daemon-reload"); got != 2 {
		t.Fatalf("daemon-reload calls = %d, want failed activation plus restored-pair reload; calls = %v", got, runner.callStrings())
	}
}

// OS-ESM-009: systemd execution history is optional runtime telemetry and a
// failed oneshot must not turn a matching timer definition into drift.
func TestApplicatorReportsFailedRuntimeWithoutChangingCheck(t *testing.T) {
	persistent := true
	root := t.TempDir()
	runner := &systemdTimerRunner{runtimeResult: "exit-code", runtimeExitCode: 42}
	provider := systemdtimer.New(models.EndpointScheduleResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}, Name: "nightly", Backend: models.ScheduleBackendSystemdTimer,
		Schedule: "daily", User: "root", Argv: []string{"/usr/bin/false"}, Persistent: &persistent,
	}, runner)
	provider.UnitDir, provider.EnvironmentDir = filepath.Join(root, "systemd"), filepath.Join(root, "environment")
	provider.LookupUser = func(string) (int, int, error) { return os.Getuid(), os.Getgid(), nil }
	provider.ValidateUnits = func(context.Context, string, string) error { return nil }
	if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("Check() = %+v, want matching configuration", check)
	}

	runtime, ok := provider.ScheduleRuntime(context.Background())
	if !ok || runtime.Status != executor.ScheduleRunFailed || runtime.ExitCode == nil || *runtime.ExitCode != 42 || runtime.MissedRunBehavior != executor.ScheduleMissedRunCatchUp {
		t.Fatalf("ScheduleRuntime() = %+v, %t", runtime, ok)
	}
}

type systemdTimerRunner struct {
	enabled              bool
	active               bool
	daemonReloadFailures int
	runtimeResult        string
	runtimeExitCode      int
	calls                []executil.MockCall
}

func (r *systemdTimerRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, executil.MockCall{Name: name, Args: append([]string(nil), args...)})
	if name != "systemctl" || len(args) == 0 {
		return nil, nil, fmt.Errorf("unexpected command %s %v", name, args)
	}
	switch args[0] {
	case "is-enabled":
		if r.enabled {
			return []byte("enabled\n"), nil, nil
		}
		return []byte("disabled\n"), nil, errors.New("disabled")
	case "is-active":
		if r.active {
			return []byte("active\n"), nil, nil
		}
		return []byte("inactive\n"), nil, errors.New("inactive")
	case "show":
		return []byte(fmt.Sprintf("Result=%s\nExecMainStatus=%d\nExecMainStartTimestampMonotonic=1234\n", r.runtimeResult, r.runtimeExitCode)), nil, nil
	case "daemon-reload":
		if r.daemonReloadFailures > 0 {
			r.daemonReloadFailures--
			return nil, []byte("synthetic reload failure"), errors.New("daemon reload failed")
		}
		return nil, nil, nil
	case "enable":
		r.enabled = true
		return nil, nil, nil
	case "start":
		r.active = true
		return nil, nil, nil
	case "disable":
		r.enabled = false
		return nil, nil, nil
	case "stop":
		r.active = false
		return nil, nil, nil
	default:
		return nil, nil, fmt.Errorf("unexpected systemctl argv %v", args)
	}
}

func (r *systemdTimerRunner) callStrings() []string {
	out := make([]string, len(r.calls))
	for i, call := range r.calls {
		out[i] = call.Name + " " + strings.Join(call.Args, " ")
	}
	return out
}
