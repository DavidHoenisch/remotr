//go:build vmsafety

package logrotate_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/applicators/logrotate"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/types"
	"github.com/DavidHoenisch/remotr/test/testsupport"
)

// OS-AEC-054: TestLogrotateProviderVM exercises the registered provider
// against Ubuntu 24.04's complete native logrotate configuration and a real
// forced rotation, including every structured script phase.
func TestLogrotateProviderVM(t *testing.T) {
	const (
		managedPath      = "/etc/logrotate.d/remotr-vm-qualified"
		unmanagedPath    = "/etc/logrotate.d/remotr-vm-unmanaged"
		invalidPath      = "/etc/logrotate.d/remotr-vm-invalid"
		logDir           = "/var/log/remotr-vm-logrotate"
		unmanagedLogDir  = "/var/log/remotr-vm-unmanaged"
		previousContent  = "/var/log/remotr-vm-logrotate/*.log {\n  daily\n  rotate 1\n}\n"
		unmanagedContent = "/var/log/remotr-vm-unmanaged/*.log {\n  weekly\n  rotate 2\n}\n"
	)
	if os.Geteuid() != 0 {
		t.Fatal("logrotate VM contract must run as root")
	}
	vmAssertLogrotateUbuntu2404(t)
	workDir := t.TempDir()
	firstAction := filepath.Join(workDir, "firstaction")
	preRotate := filepath.Join(workDir, "prerotate")
	postRotate := filepath.Join(workDir, "postrotate")
	lastAction := filepath.Join(workDir, "lastaction")
	statePath := filepath.Join(workDir, "logrotate.state")
	for _, path := range []string{managedPath, unmanagedPath, invalidPath} {
		_ = os.Remove(path)
	}
	for _, path := range []string{logDir, unmanagedLogDir} {
		_ = os.RemoveAll(path)
	}
	t.Cleanup(func() {
		for _, path := range []string{managedPath, unmanagedPath, invalidPath} {
			_ = os.Remove(path)
		}
		for _, path := range []string{logDir, unmanagedLogDir} {
			_ = os.RemoveAll(path)
		}
	})
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(unmanagedLogDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managedPath, []byte(previousContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unmanagedPath, []byte(unmanagedContent), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	stateDir := t.TempDir()
	resource := models.LogrotateResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "vm-qualified",
		Paths:        []string{logDir + "/*.log"},
		Cadence:      models.LogrotateDaily,
		Retention:    vmLogrotateInt(3),
		Compress:     vmLogrotateBool(true),
		Create:       &models.LogrotateCreate{Mode: "0640", Owner: "root", Group: "adm"},
		Shared:       vmLogrotateBool(true),
		FirstAction:  &models.LogrotateScript{Command: []string{"/usr/bin/touch", firstAction}},
		PreRotate:    &models.LogrotateScript{Command: []string{"/usr/bin/touch", preRotate}},
		PostRotate:   &models.LogrotateScript{Command: []string{"/usr/bin/touch", postRotate}},
		LastAction:   &models.LogrotateScript{Command: []string{"/usr/bin/touch", lastAction}},
	}
	provider := vmRegisteredLogrotateProvider(t, resource, stateDir, "m5-logging/logrotate")
	if err := provider.PreflightRollback(ctx); err != nil {
		t.Fatalf("logrotate rollback preflight: %v", err)
	}
	if check := provider.Check(ctx); check.Status != executor.Drifted {
		t.Fatalf("initial logrotate Check = %+v, want drifted", check)
	}

	// An invalid effective configuration containing a secret canary must fail
	// native validation without changing the active managed fragment.
	canary := testsupport.SecretCanary("ubuntu-logrotate-invalid-effective-configuration")
	if err := os.WriteFile(invalidPath, []byte(canary+" invalid effective configuration\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := provider.ApplyResult(ctx)
	if result.Status != executor.Failed || result.Err == nil || strings.Contains(result.Err.Error(), canary) {
		t.Fatalf("invalid effective configuration ApplyResult = %+v, want secret canary redacted failure", result)
	}
	if got, err := os.ReadFile(managedPath); err != nil || !bytes.Equal(got, []byte(previousContent)) {
		t.Fatalf("invalid effective configuration changed managed fragment: %q, %v", got, err)
	}
	if err := os.Remove(invalidPath); err != nil {
		t.Fatal(err)
	}

	result = provider.ApplyResult(ctx)
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional || len(result.Activation) != 0 || result.Err != nil {
		t.Fatalf("logrotate ApplyResult = %+v, want changed transactional result", result)
	}
	vmAssertLogrotateSecondCheck(t, provider)
	vmAssertLogrotateFragment(t, managedPath)
	vmRunLogrotate(t, "--debug", "/etc/logrotate.conf")
	if err := os.WriteFile(filepath.Join(logDir, "application.log"), []byte("qualified log record\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	vmRunLogrotate(t, "--force", "--state", statePath, managedPath)
	vmAssertLogrotateOperation(t, logDir, firstAction, preRotate, postRotate, lastAction)
	vmAssertLogrotateSecondCheck(t, provider)

	restarted := vmRegisteredLogrotateProvider(t, resource, stateDir, "m5-logging/logrotate")
	if err := restarted.Revert(ctx); err != nil {
		t.Fatalf("reconstructed logrotate rollback: %v", err)
	}
	if check := restarted.Check(ctx); check.Status != executor.Drifted {
		t.Fatalf("post-rollback logrotate Check = %+v, want drifted", check)
	}
	if got, err := os.ReadFile(managedPath); err != nil || !bytes.Equal(got, []byte(previousContent)) {
		t.Fatalf("logrotate rollback content = %q, %v", got, err)
	}

	if result := restarted.ApplyResult(ctx); result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("logrotate reapply = %+v, want changed", result)
	}
	vmAssertLogrotateSecondCheck(t, restarted)
	absent := models.LogrotateResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: "vm-qualified",
	}
	absentProvider := vmRegisteredLogrotateProvider(t, absent, stateDir, "m5-logging/remove-logrotate")
	if err := absentProvider.PreflightRollback(ctx); err != nil {
		t.Fatalf("logrotate removal rollback preflight: %v", err)
	}
	result = absentProvider.ApplyResult(ctx)
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional || len(result.Activation) != 0 || result.Err != nil {
		t.Fatalf("logrotate removal = %+v, want changed transactional result", result)
	}
	if check := absentProvider.Check(ctx); check.Status != executor.Compliant {
		t.Fatalf("logrotate removal second Check = %+v, want compliant", check)
	}
	if result := absentProvider.ApplyResult(ctx); result.Status != executor.NoChange || result.Err != nil {
		t.Fatalf("compliant logrotate removal = %+v, want no change", result)
	}
	if got, err := os.ReadFile(unmanagedPath); err != nil || !bytes.Equal(got, []byte(unmanagedContent)) {
		t.Fatalf("logrotate lifecycle changed unrelated fragment: %q, %v", got, err)
	}
}

func vmRegisteredLogrotateProvider(t *testing.T, resource models.LogrotateResource, stateDir, address string) *logrotate.Applicator {
	t.Helper()
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	resources, err := registry.Resources(&models.Configuration{Logrotate: []models.LogrotateResource{resource}})
	if err != nil || len(resources) != 1 || resources[0].Kind() != models.ResourceKindLogrotate {
		t.Fatalf("logrotate registry resources = %+v, %v", resources, err)
	}
	handler, err := resources[0].NewProvider(resourceregistry.FactoryContext{
		Facts: facts.Facts{Distro: types.Ubuntu, DistroVersion: "24.04", Init: facts.InitSystemd}, StateDir: stateDir,
		ArtifactDigest: "sha256:vm-logrotate", ResourceAddress: address,
	})
	provider, ok := handler.(*logrotate.Applicator)
	if err != nil || !ok {
		t.Fatalf("logrotate registry provider = %#v, %v", handler, err)
	}
	return provider
}

func vmAssertLogrotateSecondCheck(t *testing.T, provider *logrotate.Applicator) {
	t.Helper()
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("logrotate second Check = %+v, want compliant", check)
	}
	if result := provider.ApplyResult(context.Background()); result.Status != executor.NoChange || result.Err != nil {
		t.Fatalf("compliant logrotate ApplyResult = %+v, want no change", result)
	}
}

func vmAssertLogrotateFragment(t *testing.T, path string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"daily", "rotate 3", "compress", "create 0640 root adm", "sharedscripts",
		"firstaction", "prerotate", "postrotate", "lastaction",
	} {
		if !strings.Contains(string(content), marker) {
			t.Errorf("managed logrotate fragment is missing %q", marker)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("managed logrotate fragment mode = %v, want 0644", info.Mode().Perm())
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || stat.Gid != 0 {
		t.Fatalf("managed logrotate fragment ownership = %T %+v, want root:root", info.Sys(), stat)
	}
}

func vmAssertLogrotateOperation(t *testing.T, logDir string, scriptMarkers ...string) {
	t.Helper()
	for _, path := range scriptMarkers {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("logrotate script marker %q is unavailable: %v", path, err)
		}
	}
	activePath := filepath.Join(logDir, "application.log")
	info, err := os.Stat(activePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("created log file mode = %v, want 0640", info.Mode().Perm())
	}
	group, err := user.LookupGroup("adm")
	if err != nil {
		t.Fatal(err)
	}
	wantGID, err := strconv.ParseUint(group.Gid, 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || uint64(stat.Gid) != wantGID {
		t.Fatalf("created log file ownership = %T %+v, want root:adm", info.Sys(), stat)
	}
	rotatedPath := activePath + ".1.gz"
	output := vmRunLogrotate(t, "gzip", "--decompress", "--stdout", rotatedPath)
	if !strings.Contains(output, "qualified log record") {
		t.Fatalf("rotated compressed log content = %q", output)
	}
}

func vmRunLogrotate(t *testing.T, name string, args ...string) string {
	t.Helper()
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, output)
	}
	return string(output)
}

func vmAssertLogrotateUbuntu2404(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile("/etc/os-release")
	if err != nil || !strings.Contains(string(raw), "ID=ubuntu") || !strings.Contains(string(raw), `VERSION_ID="24.04"`) {
		t.Fatalf("logrotate VM OS release = %q, %v", raw, err)
	}
	if _, err := exec.LookPath("logrotate"); err != nil {
		t.Fatalf("native logrotate is unavailable: %v", err)
	}
}

func vmLogrotateBool(value bool) *bool { return &value }
func vmLogrotateInt(value int) *int    { return &value }
