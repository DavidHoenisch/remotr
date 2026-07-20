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
		"TestCoordinatedRebootSafetyVM", "remotr-vm-sysctl-safety.test", "TestSysctlProviderContractVM",
		"remotr-vm-hostname-safety.test", "TestHostnameProviderContractVM",
	} {
		if !strings.Contains(harness, marker) {
			t.Errorf("VM harness is missing %q", marker)
		}
	}
	rebootTest := readRepositoryFile(t, "internal", "applicators", "reboots", "vm_safety_test.go")
	for _, marker := range []string{"vmAssertRebootPreflight", "active_workload_inhibitor", "vmAssertSameBootTimeoutIsTerminal", "reboot_timeout_same_boot_id"} {
		if !strings.Contains(rebootTest, marker) {
			t.Errorf("reboot VM provider test is missing %q", marker)
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
	for _, marker := range []string{
		`ip -4 route replace blackhole "$control_host"/32`,
		`ip -4 route del blackhole "$control_host"/32`,
		"rolled-back",
		"Authorization: Bearer",
	} {
		if !strings.Contains(network, marker) {
			t.Errorf("network recovery fixture is missing %q", marker)
		}
	}
}

func TestNetworkRecoveryFixtureRunsHostsProviderOnPinnedUbuntu(t *testing.T) {
	harness := readRepositoryFile(t, "test", "vagrant", "harness.sh")
	start := strings.Index(harness, "network_recovery() {")
	end := strings.Index(harness, "system_safety_cleanup() {")
	if start < 0 || end <= start {
		t.Fatal("VM harness is missing the bounded network-recovery function")
	}
	fixture := harness[start:end]
	for _, marker := range []string{
		"export REMOTR_VM_BOX=cloud-image/ubuntu-24.04",
		"export REMOTR_VM_BOX_VERSION=20260705.0.0",
		"export REMOTR_VM_HOSTNAME=remotr-ubuntu-network-recovery",
		`vagrant ssh -c 'ip -4 route show default'`,
		`if ($i == "via") { print $(i + 1); exit }`,
		`test "$ID" = ubuntu; test "$VERSION_ID" = 24.04`,
		"remotr-vm-hosts-entry.test",
		"-test.run '^TestHostsEntryProviderVM$'",
	} {
		if !strings.Contains(fixture, marker) {
			t.Errorf("network-recovery VM harness is missing %q", marker)
		}
	}
	providerTest := readRepositoryFile(t, "internal", "applicators", "hostsentries", "vm_provider_test.go")
	for _, marker := range []string{
		"//go:build vmsafety",
		"func TestHostsEntryProviderVM",
		"ResourceKindHostsEntry",
		"effective",
		"LifecycleAbsent",
		"second Apply",
	} {
		if !strings.Contains(providerTest, marker) {
			t.Errorf("hosts-entry VM provider test is missing %q", marker)
		}
	}
}

func TestNetworkRecoveryFixtureRunsDNSProviderOnPinnedUbuntu(t *testing.T) {
	provision := readRepositoryFile(t, "test", "vagrant", "provision.sh")
	for _, marker := range []string{"network-manager", "[device-remotr-dns0]", "match-device=interface-name:remotr-dns0", "managed=1"} {
		if !strings.Contains(provision, marker) {
			t.Errorf("network-recovery VM provisioner is missing %q", marker)
		}
	}
	harness := readRepositoryFile(t, "test", "vagrant", "harness.sh")
	start := strings.Index(harness, "network_recovery() {")
	end := strings.Index(harness, "system_safety_cleanup() {")
	if start < 0 || end <= start {
		t.Fatal("VM harness is missing the bounded network-recovery function")
	}
	fixture := harness[start:end]
	for _, marker := range []string{
		"remotr-vm-network-resources.test",
		"-test.run '^TestDNSResolverProviderVM$'",
	} {
		if !strings.Contains(fixture, marker) {
			t.Errorf("network-recovery VM harness is missing %q", marker)
		}
	}
	providerTest := readRepositoryFile(t, "internal", "applicators", "networkresources", "vm_dns_provider_test.go")
	for _, marker := range []string{
		"//go:build vmsafety",
		"func TestDNSResolverProviderVM",
		"ResourceKindDNSResolver",
		`"ip", "link", "add", vmDNSInterface, "type", "dummy"`,
		`"ip", "link", "del", vmDNSInterface`,
		`"device", "set", vmDNSInterface, "managed", "yes"`,
		"CheckpointRollback",
		"CheckpointDestroy",
		"RollbackTransactional",
		"second Check",
	} {
		if !strings.Contains(providerTest, marker) {
			t.Errorf("DNS resolver VM provider test is missing %q", marker)
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
		"/tmp/remotr-vm-user-file-safety.test",
		`. /etc/os-release; test "$ID" = ubuntu; test "$VERSION_ID" = 24.04`,
		"-test.run '^TestGroupProviderContractVM$'",
		"-test.run '^TestUserProviderContractVM$'",
		"-test.run '^TestUserRemovalSafetyVM$'",
		"-test.run '^TestAuthorizedKeyProviderContractVM$'",
		"-test.run '^TestSudoProviderContractVM$'",
		"-test.run '^TestUserFileProviderContractVM$'",
	} {
		if !strings.Contains(userSafety, marker) {
			t.Errorf("VM harness is missing %q", marker)
		}
	}
	if !strings.Contains(harness, "user-safety) user_safety") {
		t.Error("VM harness is missing the user-safety command dispatch")
	}
}

func TestKernelModuleSafetyFixtureRunsOnPinnedUbuntu(t *testing.T) {
	harness := readRepositoryFile(t, "test", "vagrant", "harness.sh")
	start := strings.Index(harness, "kernel_module_safety() {")
	end := strings.Index(harness, "host_locale_cleanup() {")
	if start < 0 || end <= start {
		t.Fatal("VM harness is missing the bounded kernel-module-safety function")
	}
	fixture := harness[start:end]
	for _, marker := range []string{
		"export REMOTR_VM_BOX=cloud-image/ubuntu-24.04",
		"export REMOTR_VM_BOX_VERSION=20260705.0.0",
		"export REMOTR_VM_HOSTNAME=remotr-ubuntu-kernel-module-safety",
		`test "$ID" = ubuntu; test "$VERSION_ID" = 24.04`,
		"-test.run '^TestKernelModuleProviderContractVM$'",
	} {
		if !strings.Contains(fixture, marker) {
			t.Errorf("kernel-module VM harness is missing %q", marker)
		}
	}
}

func TestHostLocaleSafetyFixtureRunsOnPinnedUbuntu(t *testing.T) {
	harness := readRepositoryFile(t, "test", "vagrant", "harness.sh")
	start := strings.Index(harness, "host_locale() {")
	end := strings.Index(harness, "time_sync_cleanup() {")
	if start < 0 || end <= start {
		t.Fatal("VM harness is missing the bounded host-locale function")
	}
	fixture := harness[start:end]
	for _, marker := range []string{
		"export REMOTR_VM_BOX=cloud-image/ubuntu-24.04",
		"export REMOTR_VM_BOX_VERSION=20260705.0.0",
		"export REMOTR_VM_HOSTNAME=remotr-ubuntu-host-locale",
		`test "$ID" = ubuntu; test "$VERSION_ID" = 24.04`,
		"-test.run '^TestHostLocaleNativeKeymapValidationVM$'",
		"-test.run '^TestHostLocaleProviderVM$'",
	} {
		if !strings.Contains(fixture, marker) {
			t.Errorf("host-locale VM harness is missing %q", marker)
		}
	}
}

func TestTimeSyncSafetyFixtureRunsOnPinnedUbuntu(t *testing.T) {
	harness := readRepositoryFile(t, "test", "vagrant", "harness.sh")
	start := strings.Index(harness, "time_sync() {")
	end := strings.Index(harness, "mount_cleanup() {")
	if start < 0 || end <= start {
		t.Fatal("VM harness is missing the bounded time-sync function")
	}
	fixture := harness[start:end]
	for _, marker := range []string{
		"export REMOTR_VM_BOX=cloud-image/ubuntu-24.04",
		"export REMOTR_VM_BOX_VERSION=20260705.0.0",
		"export REMOTR_VM_HOSTNAME=remotr-ubuntu-time-sync",
		`test "$ID" = ubuntu; test "$VERSION_ID" = 24.04`,
		"-test.run '^TestTimeSyncProviderVM$'",
	} {
		if !strings.Contains(fixture, marker) {
			t.Errorf("time-sync VM harness is missing %q", marker)
		}
	}
}

func TestMountSafetyFixtureRunsOnPinnedUbuntu(t *testing.T) {
	harness := readRepositoryFile(t, "test", "vagrant", "harness.sh")
	start := strings.Index(harness, "mount_provider() {")
	end := strings.Index(harness, "failure_cleanup() {")
	if start < 0 || end <= start {
		t.Fatal("VM harness is missing the bounded mount-provider function")
	}
	fixture := harness[start:end]
	for _, marker := range []string{
		"export REMOTR_VM_BOX=cloud-image/ubuntu-24.04",
		"export REMOTR_VM_BOX_VERSION=20260705.0.0",
		"export REMOTR_VM_HOSTNAME=remotr-ubuntu-mount-safety",
		`test "$ID" = ubuntu; test "$VERSION_ID" = 24.04`,
		"-test.run '^TestMountProviderVM$'",
	} {
		if !strings.Contains(fixture, marker) {
			t.Errorf("mount VM harness is missing %q", marker)
		}
	}
}

func TestSwapSafetyFixtureRunsOnPinnedUbuntu(t *testing.T) {
	harness := readRepositoryFile(t, "test", "vagrant", "harness.sh")
	start := strings.Index(harness, "swap_provider() {")
	end := strings.Index(harness, "failure_cleanup() {")
	if start < 0 || end <= start {
		t.Fatal("VM harness is missing the bounded swap-provider function")
	}
	fixture := harness[start:end]
	for _, marker := range []string{
		"export REMOTR_VM_BOX=cloud-image/ubuntu-24.04",
		"export REMOTR_VM_BOX_VERSION=20260705.0.0",
		"export REMOTR_VM_HOSTNAME=remotr-ubuntu-swap-safety",
		`test "$ID" = ubuntu; test "$VERSION_ID" = 24.04`,
		"-test.run '^TestSwapProviderVM$'",
	} {
		if !strings.Contains(fixture, marker) {
			t.Errorf("swap VM harness is missing %q", marker)
		}
	}
}

func TestCronContainerFixtureRunsOnPinnedUbuntu(t *testing.T) {
	harness := readRepositoryFile(t, "scripts", "provider-matrix-containers.sh")
	for _, marker := range []string{
		"cron_provider_test=",
		"./internal/applicators/endpointschedules/cron",
		"$cron_provider_test:/usr/local/lib/remotr-cron-provider.test:ro",
		"TestCronProviderUbuntuContainer",
	} {
		if !strings.Contains(harness, marker) {
			t.Errorf("container harness is missing cron marker %q", marker)
		}
	}
	dockerfile := readRepositoryFile(t, "test", "provider-matrix", "containers", "Dockerfile.ubuntu-24.04")
	if !strings.Contains(dockerfile, " cron") {
		t.Error("pinned Ubuntu container does not install the native cron backend")
	}
}

func TestSystemdTimerSafetyFixtureRunsOnPinnedUbuntu(t *testing.T) {
	harness := readRepositoryFile(t, "test", "vagrant", "harness.sh")
	start := strings.Index(harness, "systemd_timer() {")
	end := strings.Index(harness, "failure_cleanup() {")
	if start < 0 || end <= start {
		t.Fatal("VM harness is missing the bounded systemd-timer function")
	}
	fixture := harness[start:end]
	for _, marker := range []string{
		"export REMOTR_VM_BOX=cloud-image/ubuntu-24.04",
		"export REMOTR_VM_BOX_VERSION=20260705.0.0",
		"export REMOTR_VM_HOSTNAME=remotr-ubuntu-systemd-timer",
		`test "$ID" = ubuntu; test "$VERSION_ID" = 24.04`,
		"-test.run '^TestSystemdTimerProviderVM$'",
	} {
		if !strings.Contains(fixture, marker) {
			t.Errorf("systemd-timer VM harness is missing %q", marker)
		}
	}
	providerTest := readRepositoryFile(t, "internal", "applicators", "endpointschedules", "systemdtimer", "vm_provider_test.go")
	for _, marker := range []string{
		"//go:build vmsafety",
		"func TestSystemdTimerProviderVM",
		"systemd-analyze",
		"Persistent=true",
		"Persistent=false",
		"LifecycleDisabled",
		"LifecycleAbsent",
		"ScheduleRuntime",
		"second Apply",
	} {
		if !strings.Contains(providerTest, marker) {
			t.Errorf("systemd-timer VM provider test is missing %q", marker)
		}
	}
}

func TestSystemdUnitSafetyFixtureRunsOnPinnedUbuntu(t *testing.T) {
	harness := readRepositoryFile(t, "test", "vagrant", "harness.sh")
	start := strings.Index(harness, "systemd_unit_provider() {")
	end := strings.Index(harness, "service_cleanup() {")
	if start < 0 || end <= start {
		t.Fatal("VM harness is missing the bounded systemd-unit function")
	}
	fixture := harness[start:end]
	for _, marker := range []string{
		"export REMOTR_VM_BOX=cloud-image/ubuntu-24.04",
		"export REMOTR_VM_BOX_VERSION=20260705.0.0",
		"export REMOTR_VM_HOSTNAME=remotr-ubuntu-systemd-unit",
		`test "$ID" = ubuntu; test "$VERSION_ID" = 24.04`,
		"-test.run '^TestSystemdUnitProviderVM$'",
	} {
		if !strings.Contains(fixture, marker) {
			t.Errorf("systemd-unit VM harness is missing %q", marker)
		}
	}
	providerTest := readRepositoryFile(t, "internal", "applicators", "systemdunits", "vm_provider_test.go")
	for _, marker := range []string{
		"//go:build vmsafety",
		"func TestSystemdUnitProviderVM",
		"ResourceKindSystemdUnit",
		"systemd-analyze",
		"CollectActivations",
		"LifecycleAbsent",
		"second Apply",
	} {
		if !strings.Contains(providerTest, marker) {
			t.Errorf("systemd-unit VM provider test is missing %q", marker)
		}
	}
}

func TestServiceSafetyFixtureRunsProviderNeutralContractOnPinnedUbuntu(t *testing.T) {
	harness := readRepositoryFile(t, "test", "vagrant", "harness.sh")
	start := strings.Index(harness, "service_provider() {")
	end := strings.Index(harness, "failure_cleanup() {")
	if start < 0 || end <= start {
		t.Fatal("VM harness is missing the bounded service-provider function")
	}
	fixture := harness[start:end]
	for _, marker := range []string{
		"export REMOTR_VM_BOX=cloud-image/ubuntu-24.04",
		"export REMOTR_VM_BOX_VERSION=20260705.0.0",
		"export REMOTR_VM_HOSTNAME=remotr-ubuntu-service",
		`test "$ID" = ubuntu; test "$VERSION_ID" = 24.04`,
		"-test.run '^TestProviderNeutralServiceVM$'",
	} {
		if !strings.Contains(fixture, marker) {
			t.Errorf("service VM harness is missing %q", marker)
		}
	}
	providerTest := readRepositoryFile(t, "internal", "applicators", "services", "vm_provider_test.go")
	for _, marker := range []string{
		"//go:build vmsafety",
		"func TestProviderNeutralServiceVM",
		"ResourceKindService",
		"ServiceResource",
		"masked",
		"second Apply",
	} {
		if !strings.Contains(providerTest, marker) {
			t.Errorf("provider-neutral service VM test is missing %q", marker)
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
