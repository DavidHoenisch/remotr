//go:build vmsafety

package apparmor_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/applicators/apparmor"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/types"
	"github.com/DavidHoenisch/remotr/test/testsupport"
)

// TestAppArmorProviderVM exercises staged validation and effective loaded mode
// through the registered provider and Ubuntu's real apparmor_parser boundary.
func TestAppArmorProviderVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("VM AppArmor provider test must run as root")
	}
	vmAssertAppArmorUbuntu2404(t)

	ctx := context.Background()
	managedProfile := "remotr_vm_qualified"
	managedPath := filepath.Join("/etc/apparmor.d", "remotr-vm-qualified")
	unmanagedProfile := "remotr_vm_unmanaged"
	unmanagedPath := filepath.Join(t.TempDir(), "unmanaged-profile")
	validContent := "profile " + managedProfile + " flags=(attach_disconnected) {\n  /usr/bin/true r,\n}\n"
	unmanagedContent := "profile " + unmanagedProfile + " flags=(attach_disconnected) {\n  /usr/bin/false r,\n}\n"
	if err := os.WriteFile(unmanagedPath, []byte(unmanagedContent), 0o644); err != nil {
		t.Fatal(err)
	}
	vmRunAppArmorParser(t, "-r", "-W", unmanagedPath)
	t.Cleanup(func() {
		_ = exec.Command("apparmor_parser", "-R", managedPath).Run()
		_ = exec.Command("apparmor_parser", "-R", unmanagedPath).Run()
		_ = os.Remove(filepath.Join("/etc/apparmor.d/disable", filepath.Base(managedPath)))
		_ = os.Remove(managedPath)
	})

	resource := models.AppArmorProfileResource{
		Name: "vm-qualified", Profile: managedProfile, Content: validContent, Mode: models.AppArmorEnforce,
	}
	provider := vmRegisteredAppArmorProvider(t, resource)
	if check := provider.Check(ctx); check.Status != executor.Drifted {
		t.Fatalf("initial AppArmor Check = %+v, want drifted", check)
	}
	if result := provider.ApplyResult(ctx); result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("AppArmor enforce ApplyResult = %+v, want changed", result)
	}
	vmAssertAppArmorSecondCheck(t, provider, models.AppArmorEnforce)
	vmAssertAppArmorLoaded(t, unmanagedProfile, models.AppArmorEnforce)

	resource.Mode = models.AppArmorComplain
	provider = vmRegisteredAppArmorProvider(t, resource)
	if result := provider.ApplyResult(ctx); result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("AppArmor complain ApplyResult = %+v, want changed", result)
	}
	vmAssertAppArmorSecondCheck(t, provider, models.AppArmorComplain)

	canary := testsupport.SecretCanary("ubuntu-apparmor-invalid-profile")
	invalid := resource
	invalid.Mode = models.AppArmorEnforce
	invalid.Content = "profile " + managedProfile + " {\n  invalid profile " + canary + "\n}\n"
	invalidProvider := vmRegisteredAppArmorProvider(t, invalid)
	result := invalidProvider.ApplyResult(ctx)
	if result.Status != executor.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "diagnostic was redacted") || strings.Contains(result.Err.Error(), canary) {
		t.Fatalf("invalid profile ApplyResult = %+v, want redacted staged-validation failure", result)
	}
	if got, err := os.ReadFile(managedPath); err != nil || !bytes.Equal(got, []byte(validContent)) {
		t.Fatalf("invalid profile changed active content: %q, %v", got, err)
	}
	vmAssertAppArmorSecondCheck(t, provider, models.AppArmorComplain)
	vmAssertAppArmorLoaded(t, unmanagedProfile, models.AppArmorEnforce)

	resource.Mode = models.AppArmorDisabled
	provider = vmRegisteredAppArmorProvider(t, resource)
	if result := provider.ApplyResult(ctx); result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("AppArmor disabled ApplyResult = %+v, want changed", result)
	}
	vmAssertAppArmorSecondCheck(t, provider, models.AppArmorDisabled)
	disablePath := filepath.Join("/etc/apparmor.d/disable", filepath.Base(managedPath))
	if target, err := os.Readlink(disablePath); err != nil || target != "../"+filepath.Base(managedPath) {
		t.Fatalf("AppArmor disable link = %q, %v", target, err)
	}
	vmAssertAppArmorLoaded(t, unmanagedProfile, models.AppArmorEnforce)

	resource.Mode = models.AppArmorEnforce
	provider = vmRegisteredAppArmorProvider(t, resource)
	if result := provider.ApplyResult(ctx); result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("AppArmor re-enable ApplyResult = %+v, want changed", result)
	}
	vmAssertAppArmorSecondCheck(t, provider, models.AppArmorEnforce)
	vmAssertAppArmorLoaded(t, unmanagedProfile, models.AppArmorEnforce)
}

func vmRegisteredAppArmorProvider(t *testing.T, resource models.AppArmorProfileResource) *apparmor.Applicator {
	t.Helper()
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	resources, err := registry.Resources(&models.Configuration{AppArmorProfiles: []models.AppArmorProfileResource{resource}})
	if err != nil || len(resources) != 1 || resources[0].Kind() != models.ResourceKindAppArmorProfile {
		t.Fatalf("AppArmor registry resources = %+v, %v", resources, err)
	}
	handler, err := resources[0].NewProvider(resourceregistry.FactoryContext{
		Facts: facts.Facts{Distro: types.Ubuntu, DistroVersion: testsupport.RequireUbuntuGuestRelease(t, "24.04", "26.04"), Security: facts.SecurityAppArmor},
	})
	provider, ok := handler.(*apparmor.Applicator)
	if err != nil || !ok {
		t.Fatalf("AppArmor registry provider = %#v, %v", handler, err)
	}
	return provider
}

func vmAssertAppArmorSecondCheck(t *testing.T, provider *apparmor.Applicator, want models.AppArmorMode) {
	t.Helper()
	check := provider.Check(context.Background())
	if check.Status != executor.Compliant || !strings.Contains(string(check.ObservedSummary), "mode="+string(want)) {
		t.Fatalf("AppArmor second Check = %+v, want %s", check, want)
	}
	if result := provider.ApplyResult(context.Background()); result.Status != executor.NoChange || result.Err != nil {
		t.Fatalf("compliant AppArmor ApplyResult = %+v, want no change", result)
	}
}

func vmAssertAppArmorLoaded(t *testing.T, profile string, mode models.AppArmorMode) {
	t.Helper()
	raw, err := os.ReadFile("/sys/kernel/security/apparmor/profiles")
	if err != nil || !strings.Contains(string(raw), profile+" ("+string(mode)+")") {
		t.Fatalf("AppArmor effective state lacks %s (%s): %v", profile, mode, err)
	}
}

func vmRunAppArmorParser(t *testing.T, args ...string) {
	t.Helper()
	if output, err := exec.Command("apparmor_parser", args...).CombinedOutput(); err != nil {
		t.Fatalf("apparmor_parser %v: %v: %s", args, err, output)
	}
}

func vmAssertAppArmorUbuntu2404(t *testing.T) {
	t.Helper()
	_ = testsupport.RequireUbuntuGuestRelease(t, "24.04", "26.04")
	if enabled, err := os.ReadFile("/sys/module/apparmor/parameters/enabled"); err != nil || strings.TrimSpace(string(enabled)) != "Y" {
		t.Fatalf("AppArmor kernel enforcement is unavailable: %q, %v", enabled, err)
	}
	if _, err := os.Stat("/sys/kernel/security/apparmor/profiles"); err != nil {
		t.Fatalf("AppArmor effective profile state is unavailable: %v", err)
	}
}
