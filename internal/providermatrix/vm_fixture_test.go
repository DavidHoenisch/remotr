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
	for _, marker := range []string{"network-recovery)", "system-safety)", "boot_before=$(boot_id)", "boot_after=$(boot_id)", "reboot_pre_ack=ready"} {
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
	for _, marker := range []string{"user_safety()", "-tags=vmsafety", "TestUserRemovalSafetyVM", "user-safety) user_safety"} {
		if !strings.Contains(harness, marker) {
			t.Errorf("VM harness is missing %q", marker)
		}
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
