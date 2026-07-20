package loginpolicy_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/loginpolicy"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
	"github.com/DavidHoenisch/remotr/test/testsupport"
)

func testPolicy() models.LoginPolicyResource {
	return models.LoginPolicyResource{
		Name:               "baseline",
		Provider:           models.LoginPolicyPAMAuthUpdate,
		Priority:           900,
		RecoveryPrincipals: []string{"recovery"},
		Rules: []models.PAMRule{
			{Section: models.PAMAuth, Control: "required", Module: "pam_faillock.so", Arguments: []string{"preauth", "deny=5", "unlock_time=900"}},
			{Section: models.PAMAccount, Control: "required", Module: "pam_faillock.so"},
		},
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Enforce: boolPointer(true)},
	}
}

// OS-AEC-052: Debian/Ubuntu PAM policy must preserve a declared recovery
// principal and validate an isolated complete stack before activating it.
func TestPAMAuthUpdateProviderValidatesFullStackAndRecoveryBeforeActivation(t *testing.T) {
	root := t.TempDir()
	profilesDir := filepath.Join(root, "pam-configs")
	pamDir := filepath.Join(root, "pam.d")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pamDir, 0o755); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(profilesDir, "remotr-baseline")
	if err := os.WriteFile(profilePath, []byte("old provider-owned profile\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pamDir, "common-auth"), []byte("auth required pam_unix.so\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := &executil.MockRunner{Next: map[string]executil.MockResult{"pam-auth-update [--package]": {}}}
	provider := loginpolicy.New(testPolicy(), runner)
	provider.ProfilesDir, provider.PAMDir = profilesDir, pamDir
	rollbackRoot := filepath.Join(root, "state", "resource-transactions")
	store, err := rollbackstore.New(rollbackstore.Options{Root: rollbackRoot})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.ConfigureRollback(store, "base/login-policy", "sha256:artifact"); err != nil {
		t.Fatal(err)
	}
	provider.LookupRecovery = func(string) error { return errors.New("recovery login unavailable") }
	validated := 0
	provider.ValidateEffective = func(context.Context, string, string) error { validated++; return nil }

	if result := provider.ApplyResult(context.Background()); result.Status != executor.Failed || !strings.Contains(result.Err.Error(), "recovery") {
		t.Fatalf("missing-recovery ApplyResult() = %+v", result)
	}
	if validated != 0 || len(runner.Calls) != 0 {
		t.Fatalf("validation=%d calls=%#v before recovery", validated, runner.Calls)
	}
	assertFileContent(t, profilePath, "old provider-owned profile\n")

	provider.LookupRecovery = func(string) error { return nil }
	provider.ValidateEffective = func(_ context.Context, stagedProfiles, stagedPAM string) error {
		validated++
		if stagedProfiles == profilesDir || stagedPAM == pamDir {
			t.Fatal("effective validation must use an isolated staged tree")
		}
		profile, err := os.ReadFile(filepath.Join(stagedProfiles, "remotr-baseline"))
		if err != nil || !strings.Contains(string(profile), "pam_faillock.so preauth deny=5 unlock_time=900") {
			t.Fatalf("staged profile = %q err=%v", profile, err)
		}
		if _, err := os.Stat(filepath.Join(stagedPAM, "common-auth")); err != nil {
			t.Fatalf("complete PAM stack was not staged: %v", err)
		}
		return errors.New("staged effective PAM stack is invalid")
	}
	if result := provider.ApplyResult(context.Background()); result.Status != executor.Failed || !strings.Contains(result.Err.Error(), "invalid") {
		t.Fatalf("invalid-stack ApplyResult() = %+v", result)
	}
	assertFileContent(t, profilePath, "old provider-owned profile\n")
	if len(runner.Calls) != 0 {
		t.Fatalf("invalid stack activation calls = %#v", runner.Calls)
	}

	provider.ValidateEffective = func(context.Context, string, string) error { validated++; return nil }
	if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional {
		t.Fatalf("valid ApplyResult() = %+v", result)
	}
	if len(runner.Calls) != 1 || runner.Calls[0].Name != "pam-auth-update" || !slices.Equal(runner.Calls[0].Args, []string{"--package"}) {
		t.Fatalf("activation calls = %#v", runner.Calls)
	}
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("second Check() = %+v", check)
	}
	restartedStore, err := rollbackstore.New(rollbackstore.Options{Root: rollbackRoot})
	if err != nil {
		t.Fatal(err)
	}
	restarted := loginpolicy.New(testPolicy(), runner)
	restarted.ProfilesDir, restarted.PAMDir = profilesDir, pamDir
	if err := restarted.ConfigureRollback(restartedStore, "base/login-policy", "sha256:artifact"); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Revert(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, profilePath, "old provider-owned profile\n")
}

func TestPAMAuthUpdateProviderRejectsMalformedEffectiveStackBeforeMutation(t *testing.T) {
	root := t.TempDir()
	profilesDir := filepath.Join(root, "pam-configs")
	pamDir := filepath.Join(root, "pam.d")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pamDir, 0o755); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(profilesDir, "remotr-baseline")
	if err := os.WriteFile(profilePath, []byte("old provider-owned profile\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pamDir, "common-auth"), []byte("auth required\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{"pam-auth-update [--package]": {}}}
	provider := loginpolicy.New(testPolicy(), runner)
	provider.ProfilesDir, provider.PAMDir = profilesDir, pamDir
	provider.LookupRecovery = func(string) error { return nil }

	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "common-auth") {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	assertFileContent(t, profilePath, "old provider-owned profile\n")
	if len(runner.Calls) != 0 {
		t.Fatalf("malformed effective stack activation calls = %#v", runner.Calls)
	}
}

func TestPAMAuthUpdateActivationFailureRestoresPriorProfileAndStack(t *testing.T) {
	root := t.TempDir()
	profilesDir := filepath.Join(root, "pam-configs")
	pamDir := filepath.Join(root, "pam.d")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pamDir, 0o755); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(profilesDir, "remotr-baseline")
	if err := os.WriteFile(profilePath, []byte("old provider-owned profile\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pamDir, "common-auth"), []byte("auth required pam_unix.so\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &failOnceRunner{}
	provider := loginpolicy.New(testPolicy(), runner)
	provider.ProfilesDir, provider.PAMDir = profilesDir, pamDir
	provider.LookupRecovery = func(string) error { return nil }

	if result := provider.ApplyResult(context.Background()); result.Status != executor.Failed {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	assertFileContent(t, profilePath, "old provider-owned profile\n")
	if runner.calls != 2 {
		t.Fatalf("pam-auth-update calls = %d, want failed activation plus recovery", runner.calls)
	}
}

// OS-AEC-052: module resolution is part of full-stack validation because
// pam-auth-update accepts profiles that name modules PAM cannot load.
func TestPAMAuthUpdateProviderRejectsUnavailableModuleBeforeMutation(t *testing.T) {
	root := t.TempDir()
	profilesDir := filepath.Join(root, "pam-configs")
	pamDir := filepath.Join(root, "pam.d")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pamDir, 0o755); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(profilesDir, "remotr-baseline")
	if err := os.WriteFile(profilePath, []byte("old provider-owned profile\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pamDir, "common-auth"), []byte("auth required pam_unix.so\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	canary := testsupport.SecretCanary("login-policy-unavailable-module")
	resource := testPolicy()
	resource.Rules = []models.PAMRule{{
		Section: models.PAMSession, Control: "optional", Module: "pam_remotr_definitely_missing.so", Arguments: []string{canary},
	}}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{"pam-auth-update [--package]": {}}}
	provider := loginpolicy.New(resource, runner)
	provider.ProfilesDir, provider.PAMDir = profilesDir, pamDir
	provider.LookupRecovery = func(string) error { return nil }

	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Failed || result.Err == nil {
		t.Fatalf("unavailable-module ApplyResult() = %+v, want failed", result)
	}
	if strings.Contains(result.Err.Error(), canary) {
		t.Fatalf("unavailable-module diagnostic leaked secret canary: %v", result.Err)
	}
	assertFileContent(t, profilePath, "old provider-owned profile\n")
	if len(runner.Calls) != 0 {
		t.Fatalf("unavailable-module activation calls = %#v", runner.Calls)
	}
}

func TestPAMAuthUpdateProviderRejectsUnavailableModuleInEffectiveStackBeforeMutation(t *testing.T) {
	root := t.TempDir()
	profilesDir := filepath.Join(root, "pam-configs")
	pamDir := filepath.Join(root, "pam.d")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pamDir, 0o755); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(profilesDir, "remotr-baseline")
	if err := os.WriteFile(profilePath, []byte("old provider-owned profile\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	canary := testsupport.SecretCanary("login-policy-effective-stack")
	stack := "auth required pam_remotr_definitely_missing.so " + canary + "\n"
	if err := os.WriteFile(filepath.Join(pamDir, "common-auth"), []byte(stack), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{"pam-auth-update [--package]": {}}}
	provider := loginpolicy.New(testPolicy(), runner)
	provider.ProfilesDir, provider.PAMDir = profilesDir, pamDir
	provider.LookupRecovery = func(string) error { return nil }

	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Failed || result.Err == nil {
		t.Fatalf("unavailable effective-stack module ApplyResult() = %+v, want failed", result)
	}
	if strings.Contains(result.Err.Error(), canary) {
		t.Fatalf("effective-stack diagnostic leaked secret canary: %v", result.Err)
	}
	assertFileContent(t, profilePath, "old provider-owned profile\n")
	if len(runner.Calls) != 0 {
		t.Fatalf("unavailable effective-stack activation calls = %#v", runner.Calls)
	}
}

func TestPAMAuthUpdateProviderAllowsExplicitlyIgnoredUnavailableModuleInEffectiveStack(t *testing.T) {
	root := t.TempDir()
	profilesDir := filepath.Join(root, "pam-configs")
	pamDir := filepath.Join(root, "pam.d")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pamDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stack := "-session optional pam_remotr_definitely_missing.so\n"
	if err := os.WriteFile(filepath.Join(pamDir, "login"), []byte(stack), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{"pam-auth-update [--package]": {}}}
	provider := loginpolicy.New(testPolicy(), runner)
	provider.ProfilesDir, provider.PAMDir = profilesDir, pamDir
	provider.LookupRecovery = func(string) error { return nil }

	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("explicitly ignored unavailable module ApplyResult() = %+v, want changed", result)
	}
	if len(runner.Calls) != 1 || runner.Calls[0].Name != "pam-auth-update" {
		t.Fatalf("explicitly ignored module activation calls = %#v", runner.Calls)
	}
}

func TestPAMAuthUpdateProviderAllowsOptionalUnavailableModuleInEffectiveStack(t *testing.T) {
	root := t.TempDir()
	profilesDir := filepath.Join(root, "pam-configs")
	pamDir := filepath.Join(root, "pam.d")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pamDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stack := "session optional pam_remotr_definitely_missing.so\n"
	if err := os.WriteFile(filepath.Join(pamDir, "login"), []byte(stack), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{"pam-auth-update [--package]": {}}}
	provider := loginpolicy.New(testPolicy(), runner)
	provider.ProfilesDir, provider.PAMDir = profilesDir, pamDir
	provider.LookupRecovery = func(string) error { return nil }

	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("optional unavailable module ApplyResult() = %+v, want changed", result)
	}
	if len(runner.Calls) != 1 || runner.Calls[0].Name != "pam-auth-update" {
		t.Fatalf("optional unavailable module activation calls = %#v", runner.Calls)
	}
}

// OS-AEC-052: pam-auth-update only composes a section when its native
// <Section>-Type header is present in the provider-owned profile.
func TestPAMAuthUpdateProviderRendersEveryAuthoredStackFamily(t *testing.T) {
	root := t.TempDir()
	profilesDir := filepath.Join(root, "pam-configs")
	pamDir := filepath.Join(root, "pam.d")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pamDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pamDir, "common-auth"), []byte("auth required pam_unix.so\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resource := testPolicy()
	resource.Rules = []models.PAMRule{
		{Section: models.PAMAuth, Control: "required", Module: "pam_unix.so"},
		{Section: models.PAMAccount, Control: "required", Module: "pam_unix.so"},
		{Section: models.PAMPassword, Control: "required", Module: "pam_unix.so"},
		{Section: models.PAMSession, Control: "optional", Module: "pam_unix.so"},
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{"pam-auth-update [--package]": {}}}
	provider := loginpolicy.New(resource, runner)
	provider.ProfilesDir, provider.PAMDir = profilesDir, pamDir
	provider.LookupRecovery = func(string) error { return nil }

	if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("multi-family ApplyResult() = %+v, want changed", result)
	}
	profile, err := os.ReadFile(filepath.Join(profilesDir, "remotr-baseline"))
	if err != nil {
		t.Fatal(err)
	}
	for _, section := range []string{
		"Auth-Type: Primary\nAuth:\n",
		"Account-Type: Primary\nAccount:\n",
		"Password-Type: Primary\nPassword:\n",
		"Session-Type: Additional\nSession:\n",
	} {
		if !strings.Contains(string(profile), section) {
			t.Errorf("provider-owned PAM profile is missing %q:\n%s", section, profile)
		}
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("%s = %q err=%v, want %q", path, got, err, want)
	}
}

func boolPointer(value bool) *bool { return &value }

type failOnceRunner struct{ calls int }

func (r *failOnceRunner) Run(string, ...string) ([]byte, []byte, error) {
	r.calls++
	if r.calls == 1 {
		return nil, []byte("candidate rejected"), errors.New("exit status 1")
	}
	return nil, nil, nil
}
