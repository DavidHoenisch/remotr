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

func readRepositoryFile(t *testing.T, elements ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"..", ".."}, elements...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
