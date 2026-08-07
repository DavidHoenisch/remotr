//go:build vmsafety

package auditrules_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/applicators/auditrules"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/types"
	"github.com/DavidHoenisch/remotr/test/testsupport"
)

// TestAuditRulesProviderVM exercises the registered provider against Ubuntu's
// real auditctl/augenrules boundaries without making immutable mode irreversible.
func TestAuditRulesProviderVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("VM audit-rules provider test must run as root")
	}
	vmAssertAuditUbuntu2404(t)

	ctx := context.Background()
	managedPath := "/etc/audit/rules.d/remotr-vm-qualified.rules"
	unmanagedPath := "/etc/audit/rules.d/remotr-vm-unmanaged.rules"
	managedRule := "-w /etc/passwd -p wa -k remotr_vm_identity"
	unmanagedRule := "-w /etc/group -p wa -k remotr_vm_unmanaged"
	if err := os.Remove(managedPath); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.WriteFile(unmanagedPath, []byte(unmanagedRule+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	vmRunAuditCommand(t, "augenrules", "--load")
	t.Cleanup(func() {
		_ = os.Remove(managedPath)
		_ = os.Remove(unmanagedPath)
		_ = exec.Command("augenrules", "--load").Run()
	})

	resource := models.AuditRulesResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "vm-qualified", Rules: []string{managedRule},
	}
	provider := vmRegisteredAuditRulesProvider(t, resource)
	if check := provider.Check(ctx); check.Status != executor.Drifted {
		t.Fatalf("initial audit-rules Check = %+v, want drifted", check)
	}
	if result := provider.ApplyResult(ctx); result.Status != executor.Changed || result.RebootRequired != executor.RebootNotRequired || result.Err != nil {
		t.Fatalf("audit-rules ApplyResult = %+v, want immediate changed", result)
	}
	vmAssertAuditSecondCheck(t, provider)
	vmAssertAuditRuleLoaded(t, unmanagedRule)

	canary := testsupport.SecretCanary("ubuntu-audit-invalid-rules")
	invalid := resource
	invalid.Rules = []string{"-a always,exit -F arch=b64 -S remotr_invalid_" + canary + " -k remotr_vm_invalid"}
	invalidProvider := vmRegisteredAuditRulesProvider(t, invalid)
	result := invalidProvider.ApplyResult(ctx)
	if result.Status != executor.Failed || result.Err == nil || strings.Contains(result.Err.Error(), canary) {
		t.Fatalf("invalid rules ApplyResult = %+v, want safe staged validation failure", result)
	}
	if got, err := os.ReadFile(managedPath); err != nil || !bytes.Equal(got, []byte(managedRule+"\n")) {
		t.Fatalf("invalid rules changed persistent fragment: %q, %v", got, err)
	}
	vmAssertAuditSecondCheck(t, provider)
	vmAssertAuditRuleLoaded(t, unmanagedRule)

	immutable := resource
	immutable.Rules = []string{managedRule, "-w /etc/shadow -p wa -k remotr_vm_identity"}
	immutableProvider := vmRegisteredAuditRulesProvider(t, immutable)
	immutableProvider.ObserveImmutable = func(context.Context) (bool, error) { return true, nil }
	result = immutableProvider.ApplyResult(ctx)
	if result.Status != executor.Changed || result.RebootRequired != executor.RebootRequired ||
		len(result.Activation) != 1 || result.Activation[0].Kind != executor.ActivationRebootRequired || result.Err != nil {
		t.Fatalf("immutable audit-rules ApplyResult = %+v, want next-boot activation", result)
	}
	if check := immutableProvider.Check(ctx); check.Status != executor.Drifted || !strings.Contains(string(check.ObservedSummary), "immutable=true") {
		t.Fatalf("immutable audit-rules Check = %+v, want persistent-only drift until reboot", check)
	}
	if err := immutableProvider.Revert(ctx); err != nil {
		t.Fatalf("restore immutable audit-rules persistence: %v", err)
	}
	vmAssertAuditSecondCheck(t, provider)

	absent := models.AuditRulesResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: "vm-qualified",
	}
	absentProvider := vmRegisteredAuditRulesProvider(t, absent)
	result = absentProvider.ApplyResult(ctx)
	if result.Status != executor.Changed || result.RebootRequired != executor.RebootNotRequired || result.Err != nil {
		t.Fatalf("audit-rules removal ApplyResult = %+v, want immediate changed", result)
	}
	if check := absentProvider.Check(ctx); check.Status != executor.Compliant {
		t.Fatalf("audit-rules removal second Check = %+v, want compliant", check)
	}
	if result := absentProvider.ApplyResult(ctx); result.Status != executor.NoChange || result.Err != nil {
		t.Fatalf("compliant audit-rules removal ApplyResult = %+v, want no change", result)
	}
	vmAssertAuditRuleLoaded(t, unmanagedRule)
}

func vmRegisteredAuditRulesProvider(t *testing.T, resource models.AuditRulesResource) *auditrules.Applicator {
	t.Helper()
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	resources, err := registry.Resources(&models.Configuration{AuditRules: []models.AuditRulesResource{resource}})
	if err != nil || len(resources) != 1 || resources[0].Kind() != models.ResourceKindAuditRules {
		t.Fatalf("audit-rules registry resources = %+v, %v", resources, err)
	}
	handler, err := resources[0].NewProvider(resourceregistry.FactoryContext{
		Facts: facts.Facts{Distro: types.Ubuntu, DistroVersion: testsupport.RequireUbuntuGuestRelease(t, "24.04", "26.04")},
	})
	provider, ok := handler.(*auditrules.Applicator)
	if err != nil || !ok {
		t.Fatalf("audit-rules registry provider = %#v, %v", handler, err)
	}
	return provider
}

func vmAssertAuditSecondCheck(t *testing.T, provider *auditrules.Applicator) {
	t.Helper()
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("audit-rules second Check = %+v, want compliant", check)
	}
	if result := provider.ApplyResult(context.Background()); result.Status != executor.NoChange || result.Err != nil {
		t.Fatalf("compliant audit-rules ApplyResult = %+v, want no change", result)
	}
}

func vmAssertAuditRuleLoaded(t *testing.T, rule string) {
	t.Helper()
	output, err := exec.Command("auditctl", "-l").CombinedOutput()
	if err != nil || !strings.Contains(string(output), rule) {
		t.Fatalf("effective audit rules lack %q: %v: %s", rule, err, output)
	}
}

func vmRunAuditCommand(t *testing.T, name string, args ...string) {
	t.Helper()
	if output, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, output)
	}
}

func vmAssertAuditUbuntu2404(t *testing.T) {
	t.Helper()
	_ = testsupport.RequireUbuntuGuestRelease(t, "24.04", "26.04")
	status, err := exec.Command("auditctl", "-s").CombinedOutput()
	if err != nil || !strings.Contains(string(status), "enabled 1") {
		t.Fatalf("mutable audit kernel state is unavailable: %v: %s", err, status)
	}
}
