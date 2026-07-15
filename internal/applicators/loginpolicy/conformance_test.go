package loginpolicy_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/loginpolicy"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestPAMAuthUpdateProviderContract(t *testing.T) {
	root := t.TempDir()
	profilesDir := filepath.Join(root, "pam-configs")
	pamDir := filepath.Join(root, "pam.d")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pamDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{"pam-auth-update [--package]": {}}}
	provider := loginpolicy.New(testPolicy(), runner)
	provider.ProfilesDir, provider.PAMDir = profilesDir, pamDir
	provider.LookupRecovery = func(string) error { return nil }

	if check := provider.Check(context.Background()); check.Status != executor.Drifted {
		t.Fatalf("initial Check() = %+v", check)
	}
	if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("second Check() = %+v", check)
	}
	if result := provider.ApplyResult(context.Background()); result.Status != executor.NoChange {
		t.Fatalf("second ApplyResult() = %+v", result)
	}

	absent := testPolicy()
	absent.Lifecycle = models.LifecycleAbsent
	absent.Rules = nil
	remover := loginpolicy.New(absent, runner)
	remover.ProfilesDir, remover.PAMDir = profilesDir, pamDir
	remover.LookupRecovery = func(string) error { return nil }
	if result := remover.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("absence ApplyResult() = %+v", result)
	}
	if check := remover.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("absence Check() = %+v", check)
	}
}
