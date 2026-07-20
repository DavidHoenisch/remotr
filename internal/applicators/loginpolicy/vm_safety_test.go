//go:build vmsafety

package loginpolicy_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/applicators/loginpolicy"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/types"
	"github.com/DavidHoenisch/remotr/test/testsupport"
)

// OS-AEC-052: TestLoginPolicyProviderVM runs the registered loginPolicy
// provider against Ubuntu 24.04's native pam-auth-update stack. It activates
// password quality, password history, and lockout modules; rejects the
// unavailable pam_lastlog.so before mutation; preserves a recovery principal
// after a failed login; and verifies transactional rollback, lifecycle
// absence, and a compliant second Check.
func TestLoginPolicyProviderVM(t *testing.T) {
	const (
		managedUser     = "remotr-vm-pam-target"
		recoveryUser    = "remotr-vm-pam-recovery"
		managedProfile  = "/usr/share/pam-configs/remotr-vm-qualified"
		unmanagedPath   = "/usr/share/pam-configs/remotr-vm-recovery"
		unmanagedPolicy = "Name: Remotr VM recovery control\nDefault: yes\nPriority: 100\nSession:\n\toptional\tpam_umask.so umask=0077\n"
	)
	if os.Geteuid() != 0 {
		t.Fatal("login-policy VM contract must run as root")
	}
	vmAssertLoginPolicyUbuntu2404(t)
	for _, username := range []string{managedUser, recoveryUser} {
		vmRemoveLoginPolicyUser(username)
		if output, err := exec.Command("useradd", "--create-home", "--shell", "/bin/sh", "--", username).CombinedOutput(); err != nil {
			t.Fatalf("create %s: %v: %s", username, err, output)
		}
	}
	managedPassword := testsupport.SecretCanary("ubuntu-login-policy-managed-password")
	recoveryPassword := testsupport.SecretCanary("ubuntu-login-policy-recovery-password")
	vmSetLoginPolicyPassword(t, managedUser, managedPassword)
	vmSetLoginPolicyPassword(t, recoveryUser, recoveryPassword)
	for _, path := range []string{managedProfile, unmanagedPath} {
		_ = os.Remove(path)
	}
	vmRunLoginPolicyCommand(t, "pam-auth-update", "--package")
	t.Cleanup(func() {
		_ = os.Remove(managedProfile)
		_ = os.Remove(unmanagedPath)
		_ = exec.Command("pam-auth-update", "--package").Run()
		_ = exec.Command("faillock", "--user", managedUser, "--reset").Run()
		vmRemoveLoginPolicyUser(managedUser)
		vmRemoveLoginPolicyUser(recoveryUser)
	})
	if err := os.WriteFile(unmanagedPath, []byte(unmanagedPolicy), 0o644); err != nil {
		t.Fatal(err)
	}
	vmRunLoginPolicyCommand(t, "pam-auth-update", "--package")
	baseline := vmSnapshotLoginPolicyStack(t)

	ctx := context.Background()
	stateDir := t.TempDir()
	// Ubuntu 24.04 does not ship pam_lastlog.so. This secret canary candidate
	// must therefore fail validation without changing the active PAM stack.
	canary := testsupport.SecretCanary("ubuntu-login-policy-last-login")
	unsupported := models.LoginPolicyResource{
		Name:               "vm-qualified",
		Provider:           models.LoginPolicyPAMAuthUpdate,
		Priority:           900,
		RecoveryPrincipals: []string{recoveryUser},
		Rules: []models.PAMRule{
			{Section: models.PAMSession, Control: "optional", Module: "pam_lastlog.so", Arguments: []string{"silent", canary}},
		},
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
	}
	unsupportedProvider := vmRegisteredLoginPolicyProvider(t, unsupported, stateDir, "m5-auth/login-policy-last-login")
	if err := unsupportedProvider.PreflightRollback(ctx); err != nil {
		t.Fatalf("unsupported login-policy rollback preflight: %v", err)
	}
	result := unsupportedProvider.ApplyResult(ctx)
	if result.Status != executor.Failed || result.Err == nil || strings.Contains(result.Err.Error(), canary) {
		t.Fatalf("unavailable last-login module ApplyResult = %+v, want redacted failure", result)
	}
	if _, err := os.Stat(managedProfile); !os.IsNotExist(err) {
		t.Fatalf("unavailable last-login module created managed profile: %v", err)
	}
	vmAssertLoginPolicyStack(t, baseline)
	vmAssertLoginPolicyAuthentication(t, recoveryUser, recoveryPassword, true)

	resource := models.LoginPolicyResource{
		Name:               "vm-qualified",
		Provider:           models.LoginPolicyPAMAuthUpdate,
		Priority:           900,
		RecoveryPrincipals: []string{recoveryUser},
		Rules: []models.PAMRule{
			{Section: models.PAMAuth, Control: "optional", Module: "pam_faillock.so", Arguments: []string{"preauth", "silent", "deny=3", "unlock_time=60"}},
			{Section: models.PAMAuth, Control: "optional", Module: "pam_faillock.so", Arguments: []string{"authfail", "deny=3", "unlock_time=60"}},
			{Section: models.PAMAccount, Control: "optional", Module: "pam_faillock.so"},
			{Section: models.PAMPassword, Control: "optional", Module: "pam_pwquality.so", Arguments: []string{"retry=3", "minlen=12"}},
			{Section: models.PAMPassword, Control: "optional", Module: "pam_pwhistory.so", Arguments: []string{"remember=5", "use_authtok"}},
		},
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
	}
	provider := vmRegisteredLoginPolicyProvider(t, resource, stateDir, "m5-auth/login-policy")
	if err := provider.PreflightRollback(ctx); err != nil {
		t.Fatalf("login-policy rollback preflight: %v", err)
	}
	if check := provider.Check(ctx); check.Status != executor.Drifted {
		t.Fatalf("initial login-policy Check = %+v, want drifted", check)
	}
	result = provider.ApplyResult(ctx)
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional || result.Err != nil {
		t.Fatalf("login-policy ApplyResult = %+v, want changed transactional", result)
	}
	vmAssertLoginPolicySecondCheck(t, provider)
	vmAssertLoginPolicyProfile(t, managedProfile)
	vmAssertLoginPolicyActiveModules(t, "pam_pwquality.so", "pam_pwhistory.so", "pam_faillock.so")

	// A failed login must not remove the declared recovery principal's access.
	vmAssertLoginPolicyAuthentication(t, managedUser, "deliberately-wrong-password", false)
	vmAssertLoginPolicyAuthentication(t, recoveryUser, recoveryPassword, true)

	restarted := vmRegisteredLoginPolicyProvider(t, resource, stateDir, "m5-auth/login-policy")
	if err := restarted.Revert(ctx); err != nil {
		t.Fatalf("reconstructed login-policy rollback: %v", err)
	}
	if check := restarted.Check(ctx); check.Status != executor.Drifted {
		t.Fatalf("post-rollback login-policy Check = %+v, want drifted", check)
	}
	if _, err := os.Stat(managedProfile); !os.IsNotExist(err) {
		t.Fatalf("login-policy rollback retained managed profile: %v", err)
	}
	vmAssertLoginPolicyStack(t, baseline)
	vmAssertLoginPolicyAuthentication(t, recoveryUser, recoveryPassword, true)

	if result := restarted.ApplyResult(ctx); result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("login-policy reapply = %+v, want changed", result)
	}
	vmAssertLoginPolicySecondCheck(t, restarted)
	absent := resource
	absent.Lifecycle = models.LifecycleAbsent
	absent.Rules = nil
	absentProvider := vmRegisteredLoginPolicyProvider(t, absent, stateDir, "m5-auth/remove-login-policy")
	if err := absentProvider.PreflightRollback(ctx); err != nil {
		t.Fatalf("login-policy removal rollback preflight: %v", err)
	}
	result = absentProvider.ApplyResult(ctx)
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional || result.Err != nil {
		t.Fatalf("login-policy removal = %+v, want changed transactional", result)
	}
	if check := absentProvider.Check(ctx); check.Status != executor.Compliant {
		t.Fatalf("login-policy removal second Check = %+v, want compliant", check)
	}
	if result := absentProvider.ApplyResult(ctx); result.Status != executor.NoChange || result.Err != nil {
		t.Fatalf("compliant login-policy removal = %+v, want no change", result)
	}
	if got, err := os.ReadFile(unmanagedPath); err != nil || !bytes.Equal(got, []byte(unmanagedPolicy)) {
		t.Fatalf("login-policy lifecycle changed recovery profile: %q, %v", got, err)
	}
	vmAssertLoginPolicyAuthentication(t, recoveryUser, recoveryPassword, true)
}

func vmRegisteredLoginPolicyProvider(t *testing.T, resource models.LoginPolicyResource, stateDir, address string) *loginpolicy.Applicator {
	t.Helper()
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	resources, err := registry.Resources(&models.Configuration{LoginPolicies: []models.LoginPolicyResource{resource}})
	if err != nil || len(resources) != 1 || resources[0].Kind() != models.ResourceKindLoginPolicy {
		t.Fatalf("login-policy registry resources = %+v, %v", resources, err)
	}
	handler, err := resources[0].NewProvider(resourceregistry.FactoryContext{
		Facts: facts.Facts{Distro: types.Ubuntu, DistroVersion: "24.04"}, StateDir: stateDir,
		ArtifactDigest: "sha256:vm-login-policy", ResourceAddress: address,
	})
	provider, ok := handler.(*loginpolicy.Applicator)
	if err != nil || !ok {
		t.Fatalf("login-policy registry provider = %#v, %v", handler, err)
	}
	return provider
}

func vmAssertLoginPolicySecondCheck(t *testing.T, provider *loginpolicy.Applicator) {
	t.Helper()
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("login-policy second Check = %+v, want compliant", check)
	}
	if result := provider.ApplyResult(context.Background()); result.Status != executor.NoChange || result.Err != nil {
		t.Fatalf("compliant login-policy ApplyResult = %+v, want no change", result)
	}
}

func vmAssertLoginPolicyProfile(t *testing.T, path string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"pam_pwquality.so", "pam_pwhistory.so", "pam_faillock.so", "deny=3", "remember=5", "minlen=12"} {
		if !strings.Contains(string(content), marker) {
			t.Errorf("managed login-policy profile is missing %q", marker)
		}
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("managed login-policy profile mode = %v, %v, want 0644", info.Mode().Perm(), err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || stat.Gid != 0 {
		t.Fatalf("managed login-policy profile ownership = %T %+v, want root:root", info.Sys(), stat)
	}
}

func vmAssertLoginPolicyActiveModules(t *testing.T, modules ...string) {
	t.Helper()
	stack := vmSnapshotLoginPolicyStack(t)
	var active strings.Builder
	for _, content := range stack {
		active.Write(content)
	}
	for _, module := range modules {
		if !strings.Contains(active.String(), module) {
			t.Errorf("active Ubuntu PAM stack is missing %q", module)
		}
	}
}

func vmSnapshotLoginPolicyStack(t *testing.T) map[string][]byte {
	t.Helper()
	stack := make(map[string][]byte)
	for _, name := range []string{"common-auth", "common-account", "common-password", "common-session", "common-session-noninteractive"} {
		path := filepath.Join("/etc/pam.d", name)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read Ubuntu PAM stack %s: %v", name, err)
		}
		stack[name] = content
	}
	return stack
}

func vmAssertLoginPolicyStack(t *testing.T, want map[string][]byte) {
	t.Helper()
	got := vmSnapshotLoginPolicyStack(t)
	for name, content := range want {
		if !bytes.Equal(got[name], content) {
			t.Errorf("Ubuntu PAM stack %s changed unexpectedly", name)
		}
	}
}

func vmAssertLoginPolicyAuthentication(t *testing.T, username, password string, wantSuccess bool) {
	t.Helper()
	command := exec.Command("pamtester", "login", username, "authenticate")
	command.Stdin = strings.NewReader(password + "\n")
	output, err := command.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("PAM authentication for recovery principal %s failed: %v: %s", username, err, output)
	}
	if !wantSuccess && err == nil {
		t.Fatalf("PAM authentication for %s unexpectedly succeeded: %s", username, output)
	}
}

func vmSetLoginPolicyPassword(t *testing.T, username, password string) {
	t.Helper()
	command := exec.Command("chpasswd")
	command.Stdin = strings.NewReader(username + ":" + password + "\n")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("set VM password for %s: %v: %s", username, err, output)
	}
}

func vmAssertLoginPolicyUbuntu2404(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile("/etc/os-release")
	if err != nil || !strings.Contains(string(raw), "ID=ubuntu") || !strings.Contains(string(raw), `VERSION_ID="24.04"`) {
		t.Fatalf("login-policy VM OS release = %q, %v", raw, err)
	}
	for _, module := range []string{"pam_pwquality.so", "pam_pwhistory.so", "pam_faillock.so"} {
		if !vmLoginPolicyModuleExists(module) {
			t.Errorf("Ubuntu 24.04 PAM module %s is unavailable", module)
		}
	}
	if vmLoginPolicyModuleExists("pam_lastlog.so") {
		t.Fatal("Ubuntu 24.04 unexpectedly provides pam_lastlog.so; update the qualification boundary")
	}
}

func vmLoginPolicyModuleExists(module string) bool {
	for _, pattern := range []string{"/usr/lib/*/security/" + module, "/lib/*/security/" + module} {
		matches, _ := filepath.Glob(pattern)
		if len(matches) > 0 {
			return true
		}
	}
	return false
}

func vmRunLoginPolicyCommand(t *testing.T, name string, args ...string) string {
	t.Helper()
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, output)
	}
	return string(output)
}

func vmRemoveLoginPolicyUser(username string) {
	if _, err := user.Lookup(username); err == nil {
		_ = exec.Command("userdel", "--remove", "--force", "--", username).Run()
	}
}
