//go:build vmsafety

package services_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
)

// OS-AEC-098: exercise ResourceKindService through its provider-neutral
// registry seam against the real Ubuntu systemd manager. The fixture proves
// enablement, activation, independent-state preservation, masked and unmasked
// convergence, native activation failure recovery, compliant second Checks,
// and no-change second Apply outcomes.
func TestProviderNeutralServiceVM(t *testing.T) {
	if os.Geteuid() != 0 {
		// test-exception: EXC-021
		t.Skip("provider-neutral service VM test runs as root in the isolated Vagrant guest")
	}
	ctx := context.Background()
	unitDir := "/usr/local/lib/systemd/system"
	service := "remotr-provider-neutral-qualification.service"
	failureService := "remotr-provider-neutral-failure.service"
	servicePath := filepath.Join(unitDir, service)
	failurePath := filepath.Join(unitDir, failureService)
	serviceUnit := []byte("[Unit]\nDescription=Remotr provider-neutral service qualification\n\n[Service]\nType=simple\nExecStart=/usr/bin/sleep infinity\n")
	failureUnit := []byte("[Unit]\nDescription=Remotr provider-neutral service failure qualification\n\n[Service]\nType=oneshot\nExecStart=/usr/bin/false\nRemainAfterExit=yes\n")
	vmRemoveServices(service, failureService)
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(servicePath, serviceUnit, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(failurePath, failureUnit, 0o644); err != nil {
		t.Fatal(err)
	}
	vmServiceCommand(t, "systemctl", "daemon-reload")
	t.Cleanup(func() {
		vmRemoveServices(service, failureService)
	})

	enabled, active, masked := true, true, false
	present := vmProviderNeutralService(t, models.ServiceResource{
		Name: "ubuntu-vm", Provider: models.ServiceProviderSystemd, Scope: models.ServiceScopeSystem,
		Service: service, Enabled: &enabled, Active: &active, Masked: &masked,
	})
	if check := present.Check(ctx); check.Status != contract.Drifted {
		t.Fatalf("initial Check = %+v, want drifted", check)
	}
	if result := present.Apply(ctx); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("enabled/active Apply = %+v, want changed", result)
	}
	vmAssertServiceState(t, service, "enabled", "active")
	vmAssertServiceUnitPreserved(t, servicePath, serviceUnit)
	vmAssertServiceSecondPass(t, present)

	active = false
	stopOnly := vmProviderNeutralService(t, models.ServiceResource{
		Name: "ubuntu-vm-stop", Provider: models.ServiceProviderSystemd, Scope: models.ServiceScopeSystem,
		Service: service, Active: &active,
	})
	if result := stopOnly.Apply(ctx); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("active-only stop Apply = %+v, want changed", result)
	}
	vmAssertServiceState(t, service, "enabled", "inactive")
	vmAssertServiceUnitPreserved(t, servicePath, serviceUnit)
	vmAssertServiceSecondPass(t, stopOnly)

	enabled, masked = false, true
	maskedProvider := vmProviderNeutralService(t, models.ServiceResource{
		Name: "ubuntu-vm-mask", Provider: models.ServiceProviderSystemd, Scope: models.ServiceScopeSystem,
		Service: service, Enabled: &enabled, Active: &active, Masked: &masked,
	})
	if result := maskedProvider.Apply(ctx); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("masked Apply = %+v, want changed", result)
	}
	vmAssertServiceState(t, service, "masked", "inactive")
	vmAssertServiceUnitPreserved(t, servicePath, serviceUnit)
	vmAssertServiceSecondPass(t, maskedProvider)

	masked = false
	unmasked := vmProviderNeutralService(t, models.ServiceResource{
		Name: "ubuntu-vm-unmask", Provider: models.ServiceProviderSystemd, Scope: models.ServiceScopeSystem,
		Service: service, Enabled: &enabled, Active: &active, Masked: &masked,
	})
	if result := unmasked.Apply(ctx); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("unmasked Apply = %+v, want changed", result)
	}
	vmAssertServiceState(t, service, "disabled", "inactive")
	vmAssertServiceSecondPass(t, unmasked)

	enabled, active = true, true
	failing := vmProviderNeutralService(t, models.ServiceResource{
		Name: "ubuntu-vm-failure", Provider: models.ServiceProviderSystemd, Scope: models.ServiceScopeSystem,
		Service: failureService, Enabled: &enabled, Active: &active,
	})
	if result := failing.Apply(ctx); result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("failing activation Apply = %+v, want failed", result)
	}
	vmAssertServiceState(t, failureService, "disabled", "inactive")
	vmAssertServiceUnitPreserved(t, failurePath, failureUnit)
}

func vmProviderNeutralService(t *testing.T, service models.ServiceResource) contract.Provider {
	t.Helper()
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	resources, err := registry.Resources(&models.Configuration{Services: []models.ServiceResource{service}})
	if err != nil {
		t.Fatal(err)
	}
	var resource resourceregistry.Resource
	found := false
	for _, candidate := range resources {
		if candidate.Kind() == models.ResourceKindService {
			resource, found = candidate, true
			break
		}
	}
	if !found {
		t.Fatal("provider-neutral service resource is absent from the registry")
	}
	if err := resource.Validate(); err != nil {
		t.Fatal(err)
	}
	handler, err := resource.NewProvider(resourceregistry.FactoryContext{Facts: facts.Facts{Init: facts.InitSystemd}})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := contract.New(handler)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func vmAssertServiceState(t *testing.T, service, unitFileState, activeState string) {
	t.Helper()
	if got := vmServiceValue(t, "systemctl", "show", service, "--property=UnitFileState", "--value"); got != unitFileState {
		t.Fatalf("%s UnitFileState = %q, want %q", service, got, unitFileState)
	}
	if got := vmServiceValue(t, "systemctl", "show", service, "--property=ActiveState", "--value"); got != activeState {
		t.Fatalf("%s ActiveState = %q, want %q", service, got, activeState)
	}
}

func vmAssertServiceUnitPreserved(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path) // #nosec G304 -- fixed VM fixture path
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("unmanaged service unit %s changed to %q, want %q", path, got, want)
	}
}

func vmAssertServiceSecondPass(t *testing.T, provider contract.Provider) {
	t.Helper()
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("second Check = %+v, want compliant", check)
	}
	if result := provider.Apply(context.Background()); result.Status != executor.NoChange || result.Err != nil {
		t.Fatalf("second Apply = %+v, want no change", result)
	}
}

func vmServiceValue(t *testing.T, name string, args ...string) string {
	t.Helper()
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func vmServiceCommand(t *testing.T, name string, args ...string) {
	t.Helper()
	if output, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, output)
	}
}

func vmRemoveServices(services ...string) {
	for _, service := range services {
		_ = exec.Command("systemctl", "disable", "--now", service).Run()
		_ = exec.Command("systemctl", "unmask", service).Run()
		_ = os.Remove(filepath.Join("/etc/systemd/system", service))
		_ = os.Remove(filepath.Join("/usr/local/lib/systemd/system", service))
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
}
