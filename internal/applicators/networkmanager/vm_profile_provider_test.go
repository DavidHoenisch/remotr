//go:build vmsafety

package networkmanager_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
	"github.com/DavidHoenisch/remotr/internal/applicators/networkmanager"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/providercontract"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
)

const (
	vmProfileInterface          = "remotr-dns0"
	vmProfilePeer               = "remotr-peer0"
	vmProfileConnection         = "remotr-profile-qualification"
	vmProfileFallbackConnection = "remotr-profile-fallback"
)

func TestNetworkManagerProfileProviderVM(t *testing.T) {
	if os.Geteuid() != 0 {
		// test-exception: EXC-036
		t.Skip("NetworkManager profile VM contract requires root")
	}
	assertProfileUbuntu2404(t)
	runner := &profileVMRecordingRunner{delegate: executil.SanitizedOSRunner{}}
	cleanupVMProfile(runner)
	t.Cleanup(func() { cleanupVMProfile(runner) })
	profileVMRun(t, runner, "ip", "link", "add", vmProfileInterface, "type", "veth", "peer", "name", vmProfilePeer)
	profileVMRun(t, runner, "nmcli", "connection", "add", "type", "ethernet", "ifname", vmProfileInterface,
		"con-name", vmProfileFallbackConnection, "connection.autoconnect", "no", "802-3-ethernet.mtu", "1400",
		"ipv4.method", "manual", "ipv4.addresses", "192.0.2.2/24", "ipv6.method", "disabled")
	profileVMRun(t, runner, "nmcli", "connection", "add", "type", "ethernet", "ifname", vmProfileInterface,
		"con-name", vmProfileConnection, "connection.autoconnect", "no", "802-3-ethernet.mtu", "1300",
		"ipv4.method", "manual", "ipv4.addresses", "192.0.2.2/24", "ipv6.method", "disabled")
	profileVMRun(t, runner, "nmcli", "device", "set", vmProfileInterface, "managed", "yes")
	profileVMRun(t, runner, "nmcli", "connection", "up", vmProfileFallbackConnection)
	fallbackConfiguration := strings.TrimSpace(string(profileVMRun(t, runner, "nmcli", "-g",
		"connection.interface-name,connection.autoconnect,802-3-ethernet.mtu,ipv4.method,ipv4.addresses,ipv6.method",
		"connection", "show", vmProfileFallbackConnection)))

	now := time.Date(2026, 7, 20, 22, 0, 0, 0, time.UTC)
	stateDir := t.TempDir()
	authorized, audit, autoConnect := true, false, true
	provider := vmProfileProvider(t, runner, stateDir, func() time.Time { return now }, models.NetworkProfileResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Enforce: &authorized},
		Name:         "vm-profile", Provider: models.NetworkProviderNetworkManager,
		Selector:    models.NetworkInterfaceSelector{Name: vmProfileInterface, Type: models.NetworkProfileEthernet},
		ProfileName: vmProfileConnection, ProfileType: models.NetworkProfileEthernet,
		AutoConnect: &autoConnect, MTU: 1450, IPv4Method: "manual", IPv6Method: "disabled",
		Addresses: []string{"192.0.2.2/24"}, Audit: &audit, RollbackTimeout: "2m",
	})
	if check := provider.Check(context.Background()); check.Status != executor.Drifted {
		t.Fatalf("initial profile Check = %+v, want drifted", check)
	}
	if err := provider.Preflight(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := provider.PreflightRollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional {
		t.Fatalf("profile ApplyResult = %+v, want changed transactional", result)
	}
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("post-Apply profile Check = %+v", check)
	}
	assertVMFallbackProfileUnchanged(t, runner, fallbackConfiguration)

	store := profileVMNetworkStore(t, stateDir, runner, provider.Now)
	status, err := store.Status()
	if err != nil || status.Intent == nil || status.Intent.Phase != networkstate.PhaseAwaitingAcknowledgement ||
		status.Intent.Interface != vmProfileInterface || status.Intent.Connection != vmProfileFallbackConnection {
		t.Fatalf("armed profile transaction = %+v, %v", status, err)
	}
	now = now.Add(3 * time.Minute)
	status, err = store.Reconcile(context.Background())
	if err != nil || status.Intent == nil || status.Intent.Phase != networkstate.PhaseRolledBack {
		t.Fatalf("timed-out profile transaction = %+v, %v", status, err)
	}
	if !profileVMRunnerCalled(runner, "CheckpointRollback") {
		t.Fatal("real NetworkManager profile checkpoint did not receive CheckpointRollback")
	}
	if active := strings.TrimSpace(string(profileVMRun(t, runner, "nmcli", "-g", "GENERAL.CONNECTION", "device", "show", vmProfileInterface))); active != vmProfileFallbackConnection {
		t.Fatalf("profile rollback restored active connection %q, want %q", active, vmProfileFallbackConnection)
	}
	if check := provider.Check(context.Background()); check.Status != executor.Drifted {
		t.Fatalf("rolled-back profile Check = %+v, want prior-state drift", check)
	}
	assertVMFallbackProfileUnchanged(t, runner, fallbackConfiguration)

	result = provider.ApplyResult(context.Background())
	if result.Status != executor.Changed || result.RollbackClass != providercontract.RollbackTransactional {
		t.Fatalf("second profile ApplyResult = %+v", result)
	}
	status, err = store.Status()
	if err != nil || status.Intent == nil {
		t.Fatalf("second armed profile transaction = %+v, %v", status, err)
	}
	if _, err := store.Acknowledge(context.Background(), status.Intent.ID); err != nil {
		t.Fatal(err)
	}
	if !profileVMRunnerCalled(runner, "CheckpointDestroy") {
		t.Fatal("authenticated profile acknowledgement did not receive CheckpointDestroy")
	}
	secondCheck := provider.Check(context.Background())
	report, ok := secondCheck.Actual.(networkmanager.ProfileReport)
	if secondCheck.Status != executor.Compliant || !ok || !report.Acknowledged || report.RollbackOutcome != "acknowledged" {
		t.Fatalf("second Check = %+v", secondCheck)
	}
	if result := provider.ApplyResult(context.Background()); result.Status != executor.NoChange || result.RollbackClass != executor.RollbackTransactional {
		t.Fatalf("compliant profile ApplyResult = %+v", result)
	}
	assertVMFallbackProfileUnchanged(t, runner, fallbackConfiguration)
}

func vmProfileProvider(t *testing.T, runner executil.Runner, stateDir string, now func() time.Time, resource models.NetworkProfileResource) *networkmanager.ProfileApplicator {
	t.Helper()
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	resources, err := registry.Resources(&models.Configuration{NetworkProfiles: []models.NetworkProfileResource{resource}})
	if err != nil || len(resources) != 1 || resources[0].Kind() != models.ResourceKindNetworkProfile {
		t.Fatalf("profile registry resources = %+v, %v", resources, err)
	}
	handler, err := resources[0].NewProvider(resourceregistry.FactoryContext{
		Facts: facts.Facts{Network: facts.NetworkManager}, Runner: runner, StateDir: stateDir,
	})
	provider, ok := handler.(*networkmanager.ProfileApplicator)
	if err != nil || !ok {
		t.Fatalf("profile registry provider = %#v, %v", handler, err)
	}
	provider.Now = now
	provider.AfterFunc = func(time.Duration, func()) {}
	return provider
}

func profileVMNetworkStore(t *testing.T, stateDir string, runner executil.Runner, now func() time.Time) *networkstate.Store {
	t.Helper()
	store, err := networkstate.New(networkstate.Options{Root: stateDir, Runner: runner, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func assertProfileUbuntu2404(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile("/etc/os-release")
	if err != nil || !strings.Contains(string(raw), "ID=ubuntu") || !strings.Contains(string(raw), `VERSION_ID="24.04"`) {
		t.Fatalf("profile VM OS release = %q, %v", raw, err)
	}
}

func profileVMRun(t *testing.T, runner executil.Runner, name string, args ...string) []byte {
	t.Helper()
	stdout, stderr, err := runner.Run(name, args...)
	if err != nil {
		t.Fatalf("%s %v: %s: %v", name, args, strings.TrimSpace(string(stderr)), err)
	}
	return stdout
}

func cleanupVMProfile(runner executil.Runner) {
	_, _, _ = runner.Run("nmcli", "connection", "delete", vmProfileConnection)
	_, _, _ = runner.Run("nmcli", "connection", "delete", vmProfileFallbackConnection)
	_, _, _ = runner.Run("ip", "link", "del", vmProfileInterface)
}

func assertVMFallbackProfileUnchanged(t *testing.T, runner executil.Runner, want string) {
	t.Helper()
	got := strings.TrimSpace(string(profileVMRun(t, runner, "nmcli", "-g",
		"connection.interface-name,connection.autoconnect,802-3-ethernet.mtu,ipv4.method,ipv4.addresses,ipv6.method",
		"connection", "show", vmProfileFallbackConnection)))
	if got != want {
		t.Fatalf("profile provider changed fallback connection: got %q, want %q", got, want)
	}
}

type profileVMRecordingRunner struct {
	delegate executil.Runner
	calls    []executil.MockCall
}

func (r *profileVMRecordingRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, executil.MockCall{Name: name, Args: append([]string(nil), args...)})
	return r.delegate.Run(name, args...)
}

func profileVMRunnerCalled(runner *profileVMRecordingRunner, operation string) bool {
	for _, call := range runner.calls {
		if call.Name == "busctl" && len(call.Args) > 4 && call.Args[4] == operation {
			return true
		}
	}
	return false
}
