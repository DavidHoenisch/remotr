//go:build vmsafety

package accountlimits_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"os/user"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/applicators/accountlimits"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// TestAccountLimitProviderVM exercises the registered accountLimit provider
// through Ubuntu's pam_limits.so session boundary. It covers invalid full
// configuration preservation, logout activation, a recovery principal,
// transactional restoration, lifecycle absence, and a compliant second Check.
func TestAccountLimitProviderVM(t *testing.T) {
	const (
		managedUser      = "remotr-vm-limit-target"
		recoveryUser     = "remotr-vm-limit-recovery"
		managedPath      = "/etc/security/limits.d/90-remotr-vm-qualified.conf"
		unmanagedPath    = "/etc/security/limits.d/89-remotr-vm-limit-recovery.conf"
		invalidPath      = "/etc/security/limits.d/88-remotr-vm-invalid.conf"
		previousContent  = "remotr-vm-limit-target soft nofile 1024\nremotr-vm-limit-target hard nofile 1024\n"
		managedContent   = "remotr-vm-limit-target soft nofile 4096\nremotr-vm-limit-target hard nofile 4096\n"
		unmanagedContent = "remotr-vm-limit-recovery soft nofile 2048\nremotr-vm-limit-recovery hard nofile 2048\n"
	)
	if os.Geteuid() != 0 {
		t.Fatal("account-limit VM contract must run as root")
	}
	vmAssertAccountLimitUbuntu2404(t)
	vmRemoveAccountLimitUser(managedUser)
	vmRemoveAccountLimitUser(recoveryUser)
	for _, username := range []string{managedUser, recoveryUser} {
		if output, err := exec.Command("useradd", "--create-home", "--shell", "/bin/sh", "--", username).CombinedOutput(); err != nil {
			t.Fatalf("create %s: %v: %s", username, err, output)
		}
	}
	for _, path := range []string{managedPath, unmanagedPath, invalidPath} {
		_ = os.Remove(path)
	}
	t.Cleanup(func() {
		for _, path := range []string{managedPath, unmanagedPath, invalidPath} {
			_ = os.Remove(path)
		}
		vmRemoveAccountLimitUser(managedUser)
		vmRemoveAccountLimitUser(recoveryUser)
	})
	if err := os.WriteFile(managedPath, []byte(previousContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unmanagedPath, []byte(unmanagedContent), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	stateDir := t.TempDir()
	resource := models.AccountLimitResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "vm-qualified",
		Entries: []models.AccountLimitEntry{
			{Domain: managedUser, Type: models.AccountLimitSoft, Item: "nofile", Value: "4096"},
			{Domain: managedUser, Type: models.AccountLimitHard, Item: "nofile", Value: "4096"},
		},
	}
	provider := vmRegisteredAccountLimitProvider(t, resource, stateDir, "m5-auth/account-limits")
	if err := provider.PreflightRollback(ctx); err != nil {
		t.Fatalf("account-limit rollback preflight: %v", err)
	}
	if check := provider.Check(ctx); check.Status != executor.Drifted {
		t.Fatalf("initial account-limit Check = %+v, want drifted", check)
	}

	if err := os.WriteFile(invalidPath, []byte("invalid full configuration\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := provider.ApplyResult(ctx)
	if result.Status != executor.Failed || result.Err == nil {
		t.Fatalf("invalid full configuration ApplyResult = %+v, want failed", result)
	}
	if got, err := os.ReadFile(managedPath); err != nil || !bytes.Equal(got, []byte(previousContent)) {
		t.Fatalf("invalid limits tree changed managed fragment: %q, %v", got, err)
	}
	vmAssertAccountLimitSession(t, recoveryUser, "2048", "2048")
	if err := os.Remove(invalidPath); err != nil {
		t.Fatal(err)
	}

	result = provider.ApplyResult(ctx)
	wantActivation := []executor.ActivationSignal{{Kind: executor.ActivationLogoutRequired}}
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional ||
		!slices.Equal(result.Activation, wantActivation) || result.Err != nil {
		t.Fatalf("account-limit ApplyResult = %+v, want changed transactional logout activation", result)
	}
	vmAssertAccountLimitSecondCheck(t, provider)
	vmAssertAccountLimitFragment(t, managedPath, managedContent)
	vmAssertAccountLimitSession(t, managedUser, "4096", "4096")
	vmAssertAccountLimitSession(t, recoveryUser, "2048", "2048")

	restarted := vmRegisteredAccountLimitProvider(t, resource, stateDir, "m5-auth/account-limits")
	if err := restarted.Revert(ctx); err != nil {
		t.Fatalf("reconstructed account-limit rollback: %v", err)
	}
	if check := restarted.Check(ctx); check.Status != executor.Drifted {
		t.Fatalf("post-rollback account-limit Check = %+v, want drifted", check)
	}
	vmAssertAccountLimitFragment(t, managedPath, previousContent)
	vmAssertAccountLimitSession(t, managedUser, "1024", "1024")
	vmAssertAccountLimitSession(t, recoveryUser, "2048", "2048")

	if result := restarted.ApplyResult(ctx); result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("account-limit reapply = %+v, want changed", result)
	}
	vmAssertAccountLimitSecondCheck(t, restarted)

	absent := models.AccountLimitResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: "vm-qualified",
	}
	absentProvider := vmRegisteredAccountLimitProvider(t, absent, stateDir, "m5-auth/remove-account-limits")
	if err := absentProvider.PreflightRollback(ctx); err != nil {
		t.Fatalf("account-limit removal rollback preflight: %v", err)
	}
	if result := absentProvider.ApplyResult(ctx); result.Status != executor.Changed ||
		result.RollbackClass != executor.RollbackTransactional || !slices.Equal(result.Activation, wantActivation) || result.Err != nil {
		t.Fatalf("account-limit removal = %+v, want changed transactional logout activation", result)
	}
	if check := absentProvider.Check(ctx); check.Status != executor.Compliant {
		t.Fatalf("account-limit removal second Check = %+v, want compliant", check)
	}
	if result := absentProvider.ApplyResult(ctx); result.Status != executor.NoChange || result.Err != nil {
		t.Fatalf("compliant account-limit removal = %+v, want no change", result)
	}
	if got, err := os.ReadFile(unmanagedPath); err != nil || !bytes.Equal(got, []byte(unmanagedContent)) {
		t.Fatalf("account-limit lifecycle changed recovery principal fragment: %q, %v", got, err)
	}
	vmAssertAccountLimitSession(t, recoveryUser, "2048", "2048")
}

func vmRegisteredAccountLimitProvider(t *testing.T, resource models.AccountLimitResource, stateDir, address string) *accountlimits.Applicator {
	t.Helper()
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	resources, err := registry.Resources(&models.Configuration{AccountLimits: []models.AccountLimitResource{resource}})
	if err != nil || len(resources) != 1 || resources[0].Kind() != models.ResourceKindAccountLimit {
		t.Fatalf("account-limit registry resources = %+v, %v", resources, err)
	}
	handler, err := resources[0].NewProvider(resourceregistry.FactoryContext{
		Facts: facts.Facts{Distro: types.Ubuntu, DistroVersion: "24.04"}, StateDir: stateDir,
		ArtifactDigest: "sha256:vm-account-limit", ResourceAddress: address,
	})
	provider, ok := handler.(*accountlimits.Applicator)
	if err != nil || !ok {
		t.Fatalf("account-limit registry provider = %#v, %v", handler, err)
	}
	return provider
}

func vmAssertAccountLimitSecondCheck(t *testing.T, provider *accountlimits.Applicator) {
	t.Helper()
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("account-limit second Check = %+v, want compliant", check)
	}
	if result := provider.ApplyResult(context.Background()); result.Status != executor.NoChange || len(result.Activation) != 0 || result.Err != nil {
		t.Fatalf("compliant account-limit ApplyResult = %+v, want no change", result)
	}
}

func vmAssertAccountLimitFragment(t *testing.T, path, content string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, []byte(content)) {
		t.Fatalf("account-limit fragment %s = %q, %v", path, got, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("account-limit fragment mode = %v, %v, want 0644", info.Mode().Perm(), err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || stat.Gid != 0 {
		t.Fatalf("account-limit fragment ownership = %T %+v, want root:root", info.Sys(), stat)
	}
}

func vmAssertAccountLimitSession(t *testing.T, username, wantSoft, wantHard string) {
	t.Helper()
	output, err := exec.Command("su", "-s", "/bin/sh", "-c", "ulimit -Sn; ulimit -Hn", "--", username).CombinedOutput()
	if err != nil {
		t.Fatalf("PAM session for recovery principal %s: %v: %s", username, err, output)
	}
	got := strings.Fields(string(output))
	want := []string{wantSoft, wantHard}
	if !slices.Equal(got, want) {
		t.Fatalf("PAM session nofile limits for %s = %v, want %v", username, got, want)
	}
}

func vmAssertAccountLimitUbuntu2404(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile("/etc/os-release")
	if err != nil || !strings.Contains(string(raw), "ID=ubuntu") || !strings.Contains(string(raw), `VERSION_ID="24.04"`) {
		t.Fatalf("account-limit VM OS release = %q, %v", raw, err)
	}
	pamStack, err := os.ReadFile("/etc/pam.d/su")
	if err != nil || !strings.Contains(string(pamStack), "pam_limits.so") {
		t.Fatalf("Ubuntu su PAM session stack lacks pam_limits.so: %v", err)
	}
}

func vmRemoveAccountLimitUser(username string) {
	if _, err := user.Lookup(username); err == nil {
		_ = exec.Command("userdel", "--remove", "--force", "--", username).Run()
	}
}
