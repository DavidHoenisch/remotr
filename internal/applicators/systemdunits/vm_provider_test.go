//go:build vmsafety

package systemdunits_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"syscall"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
)

// OS-AEC-098: exercise ResourceKindSystemdUnit through its registry seam
// against the real Ubuntu systemd manager. The fixture proves complete-unit
// and named drop-in lifecycle, native staged syntax rejection, atomic file
// replacement, coalesced daemon reload/restart activation, preservation of
// prior and unrelated state on failure/removal, and compliant second passes.
func TestSystemdUnitProviderVM(t *testing.T) {
	if os.Geteuid() != 0 {
		// test-exception: EXC-022
		t.Skip("systemd-unit VM test runs as root in the isolated Vagrant guest")
	}
	ctx := context.Background()
	unit := "remotr-systemd-unit-qualification.service"
	unitPath := filepath.Join("/etc/systemd/system", unit)
	dropIn := "20-remotr.conf"
	dropInPath := filepath.Join("/etc/systemd/system", unit+".d", dropIn)
	siblingPath := filepath.Join("/etc/systemd/system", unit+".d", "90-local.conf")
	unrelatedPath := filepath.Join("/etc/systemd/system", "remotr-systemd-unit-unrelated.service")
	resultPath := "/run/remotr-systemd-unit-qualification"
	vmRemoveSystemdUnit(unit, unitPath, dropInPath, siblingPath, unrelatedPath, resultPath)
	t.Cleanup(func() {
		vmRemoveSystemdUnit(unit, unitPath, dropInPath, siblingPath, unrelatedPath, resultPath)
	})

	unitV1 := "[Unit]\nDescription=Remotr systemd-unit qualification v1\n\n[Service]\nType=oneshot\nExecStart=/usr/bin/sh -c 'printf base-v1 > " + resultPath + "'\nRemainAfterExit=yes\n"
	dropInV1 := "[Service]\nExecStart=\nExecStart=/usr/bin/sh -c 'printf drop-in-v1 > " + resultPath + "'\n"
	if err := os.MkdirAll(filepath.Dir(dropInPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(siblingPath, []byte("[Service]\nTimeoutStopSec=45s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unrelatedPath, []byte("[Service]\nType=oneshot\nExecStart=/usr/bin/true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	full := vmSystemdUnitProvider(t, models.SystemdUnitResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Notifications: []models.Notification{{Type: models.NotificationRestart, Target: unit}}},
		Name:         "ubuntu-vm-unit", Unit: unit, Content: unitV1,
	})
	dropInProvider := vmSystemdUnitProvider(t, models.SystemdUnitResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Notifications: []models.Notification{{Type: models.NotificationRestart, Target: unit}}},
		Name:         "ubuntu-vm-drop-in", Unit: unit, DropIn: dropIn, Content: dropInV1,
	})
	for label, provider := range map[string]contract.Provider{"unit": full, "drop-in": dropInProvider} {
		if check := provider.Check(ctx); check.Status != contract.Drifted {
			t.Fatalf("initial %s Check = %+v, want drifted", label, check)
		}
	}
	unitResult := full.Apply(ctx)
	dropInResult := dropInProvider.Apply(ctx)
	for label, result := range map[string]contract.ApplyResult{"unit": unitResult, "drop-in": dropInResult} {
		if result.Status != contract.Changed || result.Err != nil {
			t.Fatalf("%s Apply = %+v, want changed", label, result)
		}
	}
	vmActivateSystemdUnit(t, unit, unitResult, dropInResult)
	vmSystemdUnitCommand(t, "systemd-analyze", "verify", unit)
	vmAssertSystemdUnitFile(t, resultPath, "drop-in-v1")
	vmAssertSystemdUnitFile(t, siblingPath, "[Service]\nTimeoutStopSec=45s\n")
	vmAssertSystemdUnitFile(t, unrelatedPath, "[Service]\nType=oneshot\nExecStart=/usr/bin/true\n")
	vmAssertSystemdUnitSecondPass(t, full)
	vmAssertSystemdUnitSecondPass(t, dropInProvider)

	priorInfo, err := os.Stat(dropInPath)
	if err != nil {
		t.Fatal(err)
	}
	dropInV2 := "[Service]\nExecStart=\nExecStart=/usr/bin/sh -c 'printf drop-in-v2 > " + resultPath + "'\n"
	updatedDropIn := vmSystemdUnitProvider(t, models.SystemdUnitResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Notifications: []models.Notification{{Type: models.NotificationRestart, Target: unit}}},
		Name:         "ubuntu-vm-drop-in-update", Unit: unit, DropIn: dropIn, Content: dropInV2,
	})
	updatedResult := updatedDropIn.Apply(ctx)
	if updatedResult.Status != contract.Changed || updatedResult.Err != nil {
		t.Fatalf("updated drop-in Apply = %+v, want changed", updatedResult)
	}
	updatedInfo, err := os.Stat(dropInPath)
	if err != nil {
		t.Fatal(err)
	}
	if priorInfo.Sys().(*syscall.Stat_t).Ino == updatedInfo.Sys().(*syscall.Stat_t).Ino {
		t.Fatal("drop-in update retained its inode, want atomic replacement")
	}
	vmActivateSystemdUnit(t, unit, updatedResult)
	vmAssertSystemdUnitFile(t, resultPath, "drop-in-v2")
	vmAssertSystemdUnitSecondPass(t, updatedDropIn)

	invalid := vmSystemdUnitProvider(t, models.SystemdUnitResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Notifications: []models.Notification{{Type: models.NotificationRestart, Target: unit}}},
		Name:         "ubuntu-vm-invalid", Unit: unit, DropIn: dropIn, Content: "[Service]\nExecStart=\nExecStart=relative-command\n",
	})
	if result := invalid.Apply(ctx); result.Status != contract.Failed || result.Err == nil || len(result.Activation) != 0 {
		t.Fatalf("invalid staged Apply = %+v, want failed without activation", result)
	}
	vmAssertSystemdUnitFile(t, dropInPath, dropInV2)
	vmAssertSystemdUnitFile(t, resultPath, "drop-in-v2")

	absentDropIn := vmSystemdUnitProvider(t, models.SystemdUnitResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent, Notifications: []models.Notification{{Type: models.NotificationRestart, Target: unit}}},
		Name:         "ubuntu-vm-drop-in-absent", Unit: unit, DropIn: dropIn,
	})
	absentDropInResult := absentDropIn.Apply(ctx)
	if absentDropInResult.Status != contract.Changed || absentDropInResult.Err != nil {
		t.Fatalf("absent drop-in Apply = %+v, want changed", absentDropInResult)
	}
	vmActivateSystemdUnit(t, unit, absentDropInResult)
	vmAssertSystemdUnitFile(t, resultPath, "base-v1")
	vmAssertSystemdUnitFile(t, siblingPath, "[Service]\nTimeoutStopSec=45s\n")
	vmAssertSystemdUnitSecondPass(t, absentDropIn)

	vmSystemdUnitCommand(t, "systemctl", "stop", unit)
	absentUnit := vmSystemdUnitProvider(t, models.SystemdUnitResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: "ubuntu-vm-unit-absent", Unit: unit,
	})
	absentUnitResult := absentUnit.Apply(ctx)
	if absentUnitResult.Status != contract.Changed || absentUnitResult.Err != nil {
		t.Fatalf("absent unit Apply = %+v, want changed", absentUnitResult)
	}
	vmActivateSystemdUnit(t, unit, absentUnitResult)
	if _, err := os.Lstat(unitPath); !os.IsNotExist(err) {
		t.Fatalf("managed unit survives absent lifecycle: %v", err)
	}
	vmAssertSystemdUnitFile(t, unrelatedPath, "[Service]\nType=oneshot\nExecStart=/usr/bin/true\n")
	vmAssertSystemdUnitSecondPass(t, absentUnit)
}

func vmSystemdUnitProvider(t *testing.T, resource models.SystemdUnitResource) contract.Provider {
	t.Helper()
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	resources, err := registry.Resources(&models.Configuration{SystemdUnits: []models.SystemdUnitResource{resource}})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range resources {
		if candidate.Kind() != models.ResourceKindSystemdUnit {
			continue
		}
		if err := candidate.Validate(); err != nil {
			t.Fatal(err)
		}
		handler, err := candidate.NewProvider(resourceregistry.FactoryContext{})
		if err != nil {
			t.Fatal(err)
		}
		provider, err := contract.New(handler)
		if err != nil {
			t.Fatal(err)
		}
		return provider
	}
	t.Fatal("systemdUnit resource is absent from the registry")
	return nil
}

func vmActivateSystemdUnit(t *testing.T, unit string, results ...executor.ApplyResult) {
	t.Helper()
	want := []executor.ActivationSignal{{Kind: executor.ActivationDaemonReload}}
	for _, result := range results {
		for _, signal := range result.Activation {
			if signal.Kind == executor.ActivationRestart {
				want = append(want, executor.ActivationSignal{Kind: executor.ActivationRestart, Target: unit})
				break
			}
		}
		if len(want) > 1 {
			break
		}
	}
	plan := executor.CollectActivations(results)
	if !slices.Equal(plan, want) {
		t.Fatalf("CollectActivations = %+v, want %+v", plan, want)
	}
	for _, signal := range plan {
		switch signal.Kind {
		case executor.ActivationDaemonReload:
			vmSystemdUnitCommand(t, "systemctl", "daemon-reload")
		case executor.ActivationRestart:
			vmSystemdUnitCommand(t, "systemctl", "restart", signal.Target)
		default:
			t.Fatalf("unexpected systemd-unit activation %+v", signal)
		}
	}
}

func vmAssertSystemdUnitSecondPass(t *testing.T, provider contract.Provider) {
	t.Helper()
	if check := provider.Check(context.Background()); check.Status != contract.Compliant {
		t.Fatalf("second Check = %+v, want compliant", check)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.NoChange || result.Err != nil || len(result.Activation) != 0 {
		t.Fatalf("second Apply = %+v, want no change", result)
	}
}

func vmAssertSystemdUnitFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path) // #nosec G304 -- fixed VM fixture paths
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

func vmSystemdUnitCommand(t *testing.T, name string, args ...string) {
	t.Helper()
	if output, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, output)
	}
}

func vmRemoveSystemdUnit(unit string, paths ...string) {
	_ = exec.Command("systemctl", "stop", unit).Run()
	for _, path := range paths {
		_ = os.Remove(path)
	}
	_ = os.Remove(filepath.Join("/etc/systemd/system", unit+".d"))
	_ = exec.Command("systemctl", "daemon-reload").Run()
}
