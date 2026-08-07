//go:build vmsafety

package journald_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/applicators/journald"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/types"
	"github.com/DavidHoenisch/remotr/test/testsupport"
)

// OS-AEC-053: TestJournaldProviderVM exercises the registered provider against
// Ubuntu 24.04's systemd-analyze and systemctl boundaries. It covers all local
// storage, retention, disk, rate, and forwarding fields without claiming
// remote delivery health.
func TestJournaldProviderVM(t *testing.T) {
	const (
		managedPath      = "/etc/systemd/journald.conf.d/90-remotr-vm-qualified.conf"
		unmanagedPath    = "/etc/systemd/journald.conf.d/80-remotr-vm-unmanaged.conf"
		invalidPath      = "/etc/systemd/journald.conf.d/70-remotr-vm-invalid.conf"
		previousContent  = "[Journal]\nRateLimitBurst=5\n"
		unmanagedContent = "[Journal]\nForwardToWall=no\n"
	)
	if os.Geteuid() != 0 {
		t.Fatal("journald VM contract must run as root")
	}
	vmAssertJournaldUbuntu2404(t)
	if err := os.MkdirAll(filepath.Dir(managedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{managedPath, unmanagedPath, invalidPath} {
		_ = os.Remove(path)
	}
	t.Cleanup(func() {
		for _, path := range []string{managedPath, unmanagedPath, invalidPath} {
			_ = os.Remove(path)
		}
		_ = exec.Command("systemctl", "restart", "systemd-journald.service").Run()
	})
	if err := os.WriteFile(managedPath, []byte(previousContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unmanagedPath, []byte(unmanagedContent), 0o644); err != nil {
		t.Fatal(err)
	}
	vmRestartJournald(t)

	ctx := context.Background()
	stateDir := t.TempDir()
	resource := models.JournaldResource{
		ResourceMeta:          models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:                  "vm-qualified",
		Storage:               models.JournaldStorageVolatile,
		MaxRetention:          "1h",
		SystemMaxUseBytes:     vmJournaldInt64(64 << 20),
		RuntimeMaxUseBytes:    vmJournaldInt64(32 << 20),
		RateLimitInterval:     "15s",
		RateLimitBurst:        vmJournaldInt(250),
		ForwardToSyslog:       vmJournaldBool(false),
		ForwardToKernelBuffer: vmJournaldBool(false),
		ForwardToConsole:      vmJournaldBool(false),
		ForwardToWall:         vmJournaldBool(false),
	}
	provider := vmRegisteredJournaldProvider(t, resource, stateDir, "m5-logging/journald")
	if err := provider.PreflightRollback(ctx); err != nil {
		t.Fatalf("journald rollback preflight: %v", err)
	}
	if check := provider.Check(ctx); check.Status != executor.Drifted {
		t.Fatalf("initial journald Check = %+v, want drifted", check)
	}

	// An invalid effective configuration containing a secret canary must fail
	// staged validation without changing the active managed drop-in.
	canary := testsupport.SecretCanary("ubuntu-journald-invalid-effective-config")
	if err := os.WriteFile(invalidPath, []byte("[Journal]\n"+canary+" invalid effective configuration\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := provider.ApplyResult(ctx)
	if result.Status != executor.Failed || result.Err == nil || strings.Contains(result.Err.Error(), canary) {
		t.Fatalf("invalid effective configuration ApplyResult = %+v, want redacted failure", result)
	}
	if got, err := os.ReadFile(managedPath); err != nil || !bytes.Equal(got, []byte(previousContent)) {
		t.Fatalf("invalid effective configuration changed managed drop-in: %q, %v", got, err)
	}
	vmAssertJournaldActive(t)
	if err := os.Remove(invalidPath); err != nil {
		t.Fatal(err)
	}

	result = provider.ApplyResult(ctx)
	wantActivation := []executor.ActivationSignal{{Kind: executor.ActivationRestart, Target: "systemd-journald.service"}}
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional ||
		!slices.Equal(result.Activation, wantActivation) || result.Err != nil {
		t.Fatalf("journald ApplyResult = %+v, want changed transactional restart", result)
	}
	vmAssertJournaldSecondCheck(t, provider)
	vmAssertJournaldDropIn(t, managedPath)
	vmRestartJournald(t)
	vmAssertJournaldEffective(t)
	vmAssertJournaldLocalRecord(t)

	restarted := vmRegisteredJournaldProvider(t, resource, stateDir, "m5-logging/journald")
	if err := restarted.Revert(ctx); err != nil {
		t.Fatalf("reconstructed journald rollback: %v", err)
	}
	vmRestartJournald(t)
	if check := restarted.Check(ctx); check.Status != executor.Drifted {
		t.Fatalf("post-rollback journald Check = %+v, want drifted", check)
	}
	if got, err := os.ReadFile(managedPath); err != nil || !bytes.Equal(got, []byte(previousContent)) {
		t.Fatalf("journald rollback content = %q, %v", got, err)
	}
	vmAssertJournaldActive(t)

	if result := restarted.ApplyResult(ctx); result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("journald reapply = %+v, want changed", result)
	}
	vmRestartJournald(t)
	vmAssertJournaldSecondCheck(t, restarted)
	absent := models.JournaldResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: "vm-qualified",
	}
	absentProvider := vmRegisteredJournaldProvider(t, absent, stateDir, "m5-logging/remove-journald")
	if err := absentProvider.PreflightRollback(ctx); err != nil {
		t.Fatalf("journald removal rollback preflight: %v", err)
	}
	result = absentProvider.ApplyResult(ctx)
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional ||
		!slices.Equal(result.Activation, wantActivation) || result.Err != nil {
		t.Fatalf("journald removal = %+v, want changed transactional restart", result)
	}
	vmRestartJournald(t)
	if check := absentProvider.Check(ctx); check.Status != executor.Compliant {
		t.Fatalf("journald removal second Check = %+v, want compliant", check)
	}
	if result := absentProvider.ApplyResult(ctx); result.Status != executor.NoChange || result.Err != nil {
		t.Fatalf("compliant journald removal = %+v, want no change", result)
	}
	if got, err := os.ReadFile(unmanagedPath); err != nil || !bytes.Equal(got, []byte(unmanagedContent)) {
		t.Fatalf("journald lifecycle changed unrelated drop-in: %q, %v", got, err)
	}
	vmAssertJournaldActive(t)
}

func vmRegisteredJournaldProvider(t *testing.T, resource models.JournaldResource, stateDir, address string) *journald.Applicator {
	t.Helper()
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	resources, err := registry.Resources(&models.Configuration{Journald: []models.JournaldResource{resource}})
	if err != nil || len(resources) != 1 || resources[0].Kind() != models.ResourceKindJournald {
		t.Fatalf("journald registry resources = %+v, %v", resources, err)
	}
	handler, err := resources[0].NewProvider(resourceregistry.FactoryContext{
		Facts: facts.Facts{Distro: types.Ubuntu, DistroVersion: testsupport.RequireUbuntuGuestRelease(t, "24.04", "26.04"), Init: facts.InitSystemd}, StateDir: stateDir,
		ArtifactDigest: "sha256:vm-journald", ResourceAddress: address,
	})
	provider, ok := handler.(*journald.Applicator)
	if err != nil || !ok {
		t.Fatalf("journald registry provider = %#v, %v", handler, err)
	}
	return provider
}

func vmAssertJournaldSecondCheck(t *testing.T, provider *journald.Applicator) {
	t.Helper()
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("journald second Check = %+v, want compliant", check)
	}
	if result := provider.ApplyResult(context.Background()); result.Status != executor.NoChange || len(result.Activation) != 0 || result.Err != nil {
		t.Fatalf("compliant journald ApplyResult = %+v, want no change", result)
	}
}

func vmAssertJournaldDropIn(t *testing.T, path string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"Storage=volatile", "MaxRetentionSec=1h", "SystemMaxUse=67108864", "RuntimeMaxUse=33554432",
		"RateLimitIntervalSec=15s", "RateLimitBurst=250", "ForwardToSyslog=no", "ForwardToKMsg=no",
		"ForwardToConsole=no", "ForwardToWall=no",
	} {
		if !strings.Contains(string(content), marker) {
			t.Errorf("managed journald drop-in is missing %q", marker)
		}
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("managed journald drop-in mode = %v, %v, want 0644", info.Mode().Perm(), err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || stat.Gid != 0 {
		t.Fatalf("managed journald drop-in ownership = %T %+v, want root:root", info.Sys(), stat)
	}
}

func vmAssertJournaldEffective(t *testing.T) {
	t.Helper()
	output := vmRunJournaldCommand(t, "systemd-analyze", "cat-config", "systemd/journald.conf")
	for _, marker := range []string{"90-remotr-vm-qualified.conf", "80-remotr-vm-unmanaged.conf", "SystemMaxUse=67108864", "ForwardToWall=no"} {
		if !strings.Contains(output, marker) {
			t.Errorf("effective journald configuration is missing %q", marker)
		}
	}
}

func vmAssertJournaldLocalRecord(t *testing.T) {
	t.Helper()
	const message = "remotr-vm-journald-active"
	vmRunJournaldCommand(t, "systemd-cat", "--identifier=remotr-vm-journald", "/usr/bin/printf", "%s\\n", message)
	vmRunJournaldCommand(t, "journalctl", "--sync")
	output := vmRunJournaldCommand(t, "journalctl", "--identifier=remotr-vm-journald", "--grep="+message, "--no-pager", "-n", "1")
	if !strings.Contains(output, message) {
		t.Fatalf("local journald record is unavailable: %s", output)
	}
}

func vmRestartJournald(t *testing.T) {
	t.Helper()
	vmRunJournaldCommand(t, "systemctl", "restart", "systemd-journald.service")
	vmAssertJournaldActive(t)
}

func vmAssertJournaldActive(t *testing.T) {
	t.Helper()
	output, err := exec.Command("systemctl", "is-active", "systemd-journald.service").CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "active" {
		t.Fatalf("systemd-journald is not active: %v: %s", err, output)
	}
}

func vmRunJournaldCommand(t *testing.T, name string, args ...string) string {
	t.Helper()
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, output)
	}
	return string(output)
}

func vmAssertJournaldUbuntu2404(t *testing.T) {
	t.Helper()
	_ = testsupport.RequireUbuntuGuestRelease(t, "24.04", "26.04")
	vmAssertJournaldActive(t)
}

func vmJournaldBool(value bool) *bool    { return &value }
func vmJournaldInt(value int) *int       { return &value }
func vmJournaldInt64(value int64) *int64 { return &value }
