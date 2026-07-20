package providermatrix

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemSafetyFixtureDeclaresRequiredEvidence(t *testing.T) {
	fixture := readRepositoryFile(t, "test", "vagrant", "fixtures", "system-safety.sh")
	for _, marker := range []string{"mountpoint -q", "modprobe loop", "sysctl=restored", "apparmor=", "recovery_principal=verified", "reboot_pre_ack=ready"} {
		if !strings.Contains(fixture, marker) {
			t.Errorf("system safety fixture is missing %q", marker)
		}
	}
	harness := readRepositoryFile(t, "test", "vagrant", "harness.sh")
	for _, marker := range []string{
		"network-recovery)", "system-safety)", "boot_before=$(boot_id)", "boot_after=$(boot_id)", "reboot_pre_ack=ready",
		"remotr-vm-reboot-safety.test", "-tags=vmsafety", "REMOTR_REBOOT_VM_PHASE=prepare", "REMOTR_REBOOT_VM_PHASE=verify",
		"TestCoordinatedRebootSafetyVM",
	} {
		if !strings.Contains(harness, marker) {
			t.Errorf("VM harness is missing %q", marker)
		}
	}
}

func TestNegativeSafetyFixtureDeclaresRequiredRecoveryEvidence(t *testing.T) {
	fixture := readRepositoryFile(t, "test", "vagrant", "fixtures", "negative-safety.sh")
	for _, marker := range []string{
		"remotr_connectivity_loss=covered-by-network-recovery",
		"ssh_sudo_lockout=blocked-before-mutation",
		"invalid_boot_state=blocked-before-mutation",
		"ambiguous_devices=blocked-before-mutation",
		"secret_canary=redacted",
	} {
		if !strings.Contains(fixture, marker) {
			t.Errorf("negative safety fixture is missing %q", marker)
		}
	}
	network := readRepositoryFile(t, "test", "vagrant", "fixtures", "network-recovery.sh")
	for _, marker := range []string{"ip -4 route replace blackhole default", "rolled-back", "Authorization: Bearer"} {
		if !strings.Contains(network, marker) {
			t.Errorf("network recovery fixture is missing %q", marker)
		}
	}
}

func TestUserSafetyFixtureRunsTheUserProviderInVM(t *testing.T) {
	harness := readRepositoryFile(t, "test", "vagrant", "harness.sh")
	start := strings.Index(harness, "user_safety() {")
	end := strings.Index(harness, "login_policy_safety_cleanup() {")
	if start < 0 || end <= start {
		t.Fatal("VM harness is missing the bounded user-safety function")
	}
	userSafety := harness[start:end]
	for _, marker := range []string{
		"export REMOTR_VM_BOX=cloud-image/ubuntu-24.04",
		"export REMOTR_VM_BOX_VERSION=20260705.0.0",
		"export REMOTR_VM_HOSTNAME=remotr-ubuntu-user-safety",
		"user_safety()",
		"CGO_ENABLED=0 go test -mod=vendor -tags=vmsafety -c",
		"vagrant upload",
		"/tmp/remotr-vm-user-safety.test",
		"/tmp/remotr-vm-group-safety.test",
		"/tmp/remotr-vm-authorized-key-safety.test",
		"/tmp/remotr-vm-sudo-safety.test",
		`. /etc/os-release; test "$ID" = ubuntu; test "$VERSION_ID" = 24.04`,
		"-test.run '^TestGroupProviderContractVM$'",
		"-test.run '^TestUserProviderContractVM$'",
		"-test.run '^TestUserRemovalSafetyVM$'",
		"-test.run '^TestAuthorizedKeyProviderContractVM$'",
		"-test.run '^TestSudoProviderContractVM$'",
	} {
		if !strings.Contains(userSafety, marker) {
			t.Errorf("VM harness is missing %q", marker)
		}
	}
	if !strings.Contains(harness, "user-safety) user_safety") {
		t.Error("VM harness is missing the user-safety command dispatch")
	}
}

func readRepositoryFile(t *testing.T, elements ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"..", ".."}, elements...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
