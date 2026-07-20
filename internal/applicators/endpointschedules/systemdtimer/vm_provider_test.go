//go:build vmsafety

package systemdtimer_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/endpointschedules/systemdtimer"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

// OS-AEC-098 / OS-ESM-007 / OS-ESM-009: exercise the paired schedule units
// through the real Ubuntu systemd manager. The fixture proves native staged
// syntax validation, daemon reload, enablement and activation, explicit
// missed-run policy, disabled and absent lifecycles, compliant second Checks,
// and no-change replay.
func TestSystemdTimerProviderVM(t *testing.T) {
	if os.Geteuid() != 0 {
		// test-exception: EXC-020
		t.Skip("systemd-timer VM test runs as root in the isolated Vagrant guest")
	}
	ctx := context.Background()
	name := "ubuntu-vm-qualification"
	unitDir := "/etc/systemd/system"
	environmentDir := "/var/lib/remotr/schedules"
	servicePath := filepath.Join(unitDir, "remotr-schedule-"+name+".service")
	timerPath := filepath.Join(unitDir, "remotr-schedule-"+name+".timer")
	environmentPath := filepath.Join(environmentDir, name+".env")
	vmRemoveSystemdTimer(name, servicePath, timerPath, environmentPath)
	t.Cleanup(func() {
		vmRemoveSystemdTimer(name, servicePath, timerPath, environmentPath)
	})

	persistent := true
	resource := models.EndpointScheduleResource{
		ResourceMeta:     models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:             name,
		Backend:          models.ScheduleBackendSystemdTimer,
		Schedule:         "*-*-* 04:17:00",
		User:             "nobody",
		Argv:             []string{"/usr/bin/true"},
		WorkingDirectory: "/tmp",
		Environment:      []models.ScheduleEnvironment{{Name: "REMOTR_QUALIFICATION", Value: "ubuntu-2404"}},
		Timeout:          "30s",
		Overlap:          models.ScheduleOverlapForbid,
		Persistent:       &persistent,
	}
	present, presentHandler := vmSystemdTimerProvider(t, resource)
	if check := present.Check(ctx); check.Status != contract.Drifted {
		t.Fatalf("initial Check = %+v, want drifted", check)
	}
	if result := present.Apply(ctx); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("present Apply = %+v, want changed", result)
	}
	vmAssertSystemdTimerState(t, name, "enabled", "active")
	vmAssertFileContains(t, timerPath, "Persistent=true")
	vmAssertFileContains(t, servicePath, "User=nobody")
	vmAssertFileContains(t, servicePath, "EnvironmentFile="+environmentPath)
	vmAssertFileContains(t, environmentPath, `REMOTR_QUALIFICATION="ubuntu-2404"`)
	vmSystemdTimerCommand(t, "systemd-analyze", "verify", servicePath, timerPath)
	vmAssertSystemdTimerSecondPass(t, present)

	vmSystemdTimerCommand(t, "systemctl", "start", "remotr-schedule-"+name+".service")
	vmAssertMissedRunBehavior(t, presentHandler, executor.ScheduleMissedRunCatchUp)

	disabledResource := resource
	disabledResource.Lifecycle = models.LifecycleDisabled
	disabled, _ := vmSystemdTimerProvider(t, disabledResource)
	if result := disabled.Apply(ctx); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("disabled Apply = %+v, want changed", result)
	}
	vmAssertSystemdTimerState(t, name, "disabled", "inactive")
	if _, err := os.Stat(servicePath); err != nil {
		t.Fatalf("disabled lifecycle removed service unit: %v", err)
	}
	if _, err := os.Stat(timerPath); err != nil {
		t.Fatalf("disabled lifecycle removed timer unit: %v", err)
	}
	vmAssertSystemdTimerSecondPass(t, disabled)

	persistent = false
	nonPersistentResource := resource
	nonPersistentResource.Persistent = &persistent
	nonPersistent, nonPersistentHandler := vmSystemdTimerProvider(t, nonPersistentResource)
	if result := nonPersistent.Apply(ctx); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("non-persistent Apply = %+v, want changed", result)
	}
	vmAssertSystemdTimerState(t, name, "enabled", "active")
	vmAssertFileContains(t, timerPath, "Persistent=false")
	vmAssertSystemdTimerSecondPass(t, nonPersistent)
	vmSystemdTimerCommand(t, "systemctl", "start", "remotr-schedule-"+name+".service")
	vmAssertMissedRunBehavior(t, nonPersistentHandler, executor.ScheduleMissedRunSkip)

	absentResource := models.EndpointScheduleResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent},
		Name:         name,
		Backend:      models.ScheduleBackendSystemdTimer,
	}
	absent, _ := vmSystemdTimerProvider(t, absentResource)
	if result := absent.Apply(ctx); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("absent Apply = %+v, want changed", result)
	}
	for _, path := range []string{servicePath, timerPath, environmentPath} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("managed path %s survives absent lifecycle: %v", path, err)
		}
	}
	vmAssertSystemdTimerSecondPass(t, absent)
}

func vmSystemdTimerProvider(t *testing.T, resource models.EndpointScheduleResource) (contract.Provider, *systemdtimer.Applicator) {
	t.Helper()
	if err := resource.Validate(); err != nil {
		t.Fatal(err)
	}
	handler := systemdtimer.New(resource, nil)
	provider, err := contract.New(handler)
	if err != nil {
		t.Fatal(err)
	}
	return provider, handler
}

func vmAssertSystemdTimerState(t *testing.T, name, unitFileState, activeState string) {
	t.Helper()
	unit := "remotr-schedule-" + name + ".timer"
	if got := vmSystemdTimerValue(t, "systemctl", "show", unit, "--property=UnitFileState", "--value"); got != unitFileState {
		t.Fatalf("timer UnitFileState = %q, want %q", got, unitFileState)
	}
	if got := vmSystemdTimerValue(t, "systemctl", "show", unit, "--property=ActiveState", "--value"); got != activeState {
		t.Fatalf("timer ActiveState = %q, want %q", got, activeState)
	}
}

func vmAssertSystemdTimerSecondPass(t *testing.T, provider contract.Provider) {
	t.Helper()
	if check := provider.Check(context.Background()); check.Status != contract.Compliant {
		t.Fatalf("second Check = %+v, want compliant", check)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.NoChange || result.Err != nil {
		t.Fatalf("second Apply = %+v, want no change", result)
	}
}

func vmAssertMissedRunBehavior(t *testing.T, handler *systemdtimer.Applicator, want executor.ScheduleMissedRunBehavior) {
	t.Helper()
	runtime, ok := executor.ScheduleRuntime(context.Background(), handler)
	if !ok || runtime.Status != executor.ScheduleRunSucceeded || runtime.ExitCode == nil || *runtime.ExitCode != 0 || runtime.MissedRunBehavior != want {
		t.Fatalf("ScheduleRuntime = %+v, %t; want successful run with missed-run policy %q", runtime, ok, want)
	}
}

func vmAssertFileContains(t *testing.T, path, want string) {
	t.Helper()
	contents, err := os.ReadFile(path) // #nosec G304 -- fixed VM fixture paths
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), want) {
		t.Fatalf("%s = %q, want content %q", path, contents, want)
	}
}

func vmSystemdTimerValue(t *testing.T, name string, args ...string) string {
	t.Helper()
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func vmSystemdTimerCommand(t *testing.T, name string, args ...string) {
	t.Helper()
	if output, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, output)
	}
}

func vmRemoveSystemdTimer(name string, paths ...string) {
	unit := "remotr-schedule-" + name + ".timer"
	_ = exec.Command("systemctl", "disable", "--now", unit).Run()
	for _, path := range paths {
		_ = os.Remove(path)
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
}
