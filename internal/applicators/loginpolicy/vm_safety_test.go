//go:build vmsafety

package loginpolicy_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/loginpolicy"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-AEC-052: this isolated Debian VM test activates a real pam-auth-update
// profile, exercises the declared recovery principal through the resulting
// PAM-backed su stack, and proves recovery still works after rollback. This is
// technical stack/recovery verification; it does not claim a human login.
func TestLoginPolicyRecoverySafetyVM(t *testing.T) {
	if os.Geteuid() != 0 {
		// test-exception: EXC-016
		t.Skip("login-policy VM test runs as root in the isolated Vagrant guest")
	}
	const recovery = "remotr-vm-pam-recovery"
	if err := exec.Command("useradd", "--create-home", "--shell", "/bin/sh", recovery).Run(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Remove("/usr/share/pam-configs/remotr-vm-recovery")
		_ = exec.Command("pam-auth-update", "--package").Run()
		_ = exec.Command("userdel", "--remove", recovery).Run()
	})

	provider := loginpolicy.New(models.LoginPolicyResource{
		Name:               "vm-recovery",
		Provider:           models.LoginPolicyPAMAuthUpdate,
		Priority:           900,
		RecoveryPrincipals: []string{recovery},
		Rules: []models.PAMRule{
			{Section: models.PAMSession, Control: "optional", Module: "pam_umask.so", Arguments: []string{"umask=0077"}},
		},
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
	})
	if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("real pam-auth-update ApplyResult() = %+v", result)
	}
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("real provider second Check() = %+v", check)
	}
	assertRecoveryPrincipalVM(t, recovery)
	if err := provider.Revert(context.Background()); err != nil {
		t.Fatalf("real provider rollback = %v", err)
	}
	assertRecoveryPrincipalVM(t, recovery)
}

func assertRecoveryPrincipalVM(t *testing.T, principal string) {
	t.Helper()
	output, err := exec.Command("su", "-s", "/bin/sh", "-c", "id -un", principal).CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != principal {
		t.Fatalf("technical recovery path for %q failed: %q, %v", principal, output, err)
	}
}
