//go:build vmsafety

package networkresources_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
	"github.com/DavidHoenisch/remotr/internal/applicators/networkresources"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/providercontract"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
)

const (
	vmDNSConnection = "remotr-dns-qualification"
	vmDNSInterface  = "remotr-dns0"
)

func TestDNSResolverProviderVM(t *testing.T) {
	if os.Geteuid() != 0 {
		// test-exception: EXC-038
		t.Skip("DNS resolver VM contract requires root")
	}
	assertUbuntu2404(t)
	runner := &vmRecordingRunner{delegate: executil.SanitizedOSRunner{}}
	cleanupVMNetworkDevice(runner)
	t.Cleanup(func() { cleanupVMNetworkDevice(runner) })
	vmRun(t, runner, "ip", "link", "add", vmDNSInterface, "type", "dummy")
	vmRun(t, runner, "nmcli", "connection", "add", "type", "dummy", "ifname", vmDNSInterface,
		"con-name", vmDNSConnection, "ipv4.method", "manual", "ipv4.addresses", "192.0.2.2/24",
		"ipv4.ignore-auto-dns", "yes", "ipv4.dns", "198.51.100.53", "ipv4.dns-search", "old.example",
		"ipv6.method", "disabled")
	vmRun(t, runner, "nmcli", "device", "set", vmDNSInterface, "managed", "yes")
	vmRun(t, runner, "nmcli", "connection", "up", vmDNSConnection)
	originalAddressing := strings.TrimSpace(string(vmRun(t, runner, "nmcli", "-g", "ipv4.method,ipv4.addresses", "connection", "show", vmDNSConnection)))
	controlGateway := vmDefaultGateway(t, runner)

	now := time.Date(2026, 7, 20, 16, 0, 0, 0, time.UTC)
	stateDir := t.TempDir()
	authorized := true
	provider := vmDNSProvider(t, runner, stateDir, fmt.Sprintf("https://%s:8443/v1/sync", controlGateway), func() time.Time { return now }, models.DNSResolverResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Enforce: &authorized},
		Name:         "vm-dns", Provider: models.NetworkProviderNetworkManager, Interface: vmDNSInterface,
		Servers: []string{"192.0.2.53"}, SearchDomains: []string{"corp.example"}, Configured: true, Effective: true,
	})
	if check := provider.Check(context.Background()); check.Status != executor.Drifted {
		t.Fatalf("initial DNS Check = %+v, want drifted", check)
	}
	if err := provider.Preflight(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := provider.PreflightRollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional {
		t.Fatalf("DNS ApplyResult = %+v, want changed transactional", result)
	}
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("post-Apply DNS Check = %+v", check)
	}
	assertVMAddressingUnchanged(t, runner, originalAddressing)

	store := vmNetworkStore(t, stateDir, runner, provider.Now)
	status, err := store.Status()
	if err != nil || status.Intent == nil || status.Intent.Phase != networkstate.PhaseAwaitingAcknowledgement {
		t.Fatalf("armed DNS transaction = %+v, %v", status, err)
	}
	now = now.Add(3 * time.Minute)
	status, err = store.Reconcile(context.Background())
	if err != nil || status.Intent == nil || status.Intent.Phase != networkstate.PhaseRolledBack {
		t.Fatalf("timed-out DNS transaction = %+v, %v", status, err)
	}
	if !vmRunnerCalled(runner, "CheckpointRollback") {
		t.Fatal("real NetworkManager checkpoint did not receive CheckpointRollback")
	}
	if check := provider.Check(context.Background()); check.Status != executor.Drifted {
		t.Fatalf("rolled-back DNS Check = %+v, want prior-state drift", check)
	}
	assertVMAddressingUnchanged(t, runner, originalAddressing)

	result = provider.ApplyResult(context.Background())
	if result.Status != executor.Changed || result.RollbackClass != providercontract.RollbackTransactional {
		t.Fatalf("second DNS ApplyResult = %+v", result)
	}
	status, err = store.Status()
	if err != nil || status.Intent == nil {
		t.Fatalf("second armed DNS transaction = %+v, %v", status, err)
	}
	if _, err := store.Acknowledge(context.Background(), status.Intent.ID); err != nil {
		t.Fatal(err)
	}
	if !vmRunnerCalled(runner, "CheckpointDestroy") {
		t.Fatal("authenticated DNS acknowledgement did not receive CheckpointDestroy")
	}
	secondCheck := provider.Check(context.Background())
	report, ok := secondCheck.Actual.(networkresources.DNSStateReport)
	if secondCheck.Status != executor.Compliant || !ok || !report.Acknowledged || report.RollbackOutcome != "acknowledged" {
		t.Fatalf("second Check = %+v", secondCheck)
	}
	if result := provider.ApplyResult(context.Background()); result.Status != executor.NoChange || result.RollbackClass != executor.RollbackTransactional {
		t.Fatalf("compliant DNS ApplyResult = %+v", result)
	}
	assertVMAddressingUnchanged(t, runner, originalAddressing)
}

func vmDNSProvider(t *testing.T, runner executil.Runner, stateDir, syncURL string, now func() time.Time, resource models.DNSResolverResource) *networkresources.DNSApplicator {
	t.Helper()
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	resources, err := registry.Resources(&models.Configuration{DNSResolvers: []models.DNSResolverResource{resource}})
	if err != nil || len(resources) != 1 || resources[0].Kind() != models.ResourceKindDNSResolver {
		t.Fatalf("DNS registry resources = %+v, %v", resources, err)
	}
	handler, err := resources[0].NewProvider(resourceregistry.FactoryContext{
		Facts: facts.Facts{Network: facts.NetworkManager}, Runner: runner, StateDir: stateDir, SyncURL: syncURL,
	})
	provider, ok := handler.(*networkresources.DNSApplicator)
	if err != nil || !ok {
		t.Fatalf("DNS registry provider = %#v, %v", handler, err)
	}
	provider.Now = now
	provider.AfterFunc = func(time.Duration, func()) {}
	return provider
}

func vmNetworkStore(t *testing.T, stateDir string, runner executil.Runner, now func() time.Time) *networkstate.Store {
	t.Helper()
	store, err := networkstate.New(networkstate.Options{Root: stateDir, Runner: runner, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func assertUbuntu2404(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile("/etc/os-release")
	if err != nil || !strings.Contains(string(raw), "ID=ubuntu") || !strings.Contains(string(raw), `VERSION_ID="24.04"`) {
		t.Fatalf("DNS VM OS release = %q, %v", raw, err)
	}
}

func vmRun(t *testing.T, runner executil.Runner, name string, args ...string) []byte {
	t.Helper()
	stdout, stderr, err := runner.Run(name, args...)
	if err != nil {
		t.Fatalf("%s %v: %s: %v", name, args, strings.TrimSpace(string(stderr)), err)
	}
	return stdout
}

func cleanupVMNetworkDevice(runner executil.Runner) {
	_, _, _ = runner.Run("nmcli", "connection", "delete", vmDNSConnection)
	_, _, _ = runner.Run("ip", "link", "del", vmDNSInterface)
}

func vmDefaultGateway(t *testing.T, runner executil.Runner) string {
	t.Helper()
	fields := strings.Fields(string(vmRun(t, runner, "ip", "-4", "route", "show", "default")))
	for index, field := range fields {
		if field == "via" && index+1 < len(fields) {
			return fields[index+1]
		}
	}
	t.Fatal("VM has no default gateway for DNS control-path preflight")
	return ""
}

func assertVMAddressingUnchanged(t *testing.T, runner executil.Runner, want string) {
	t.Helper()
	got := strings.TrimSpace(string(vmRun(t, runner, "nmcli", "-g", "ipv4.method,ipv4.addresses", "connection", "show", vmDNSConnection)))
	if got != want {
		t.Fatalf("DNS-only provider changed addressing: got %q, want %q", got, want)
	}
}

type vmRecordingRunner struct {
	delegate executil.Runner
	calls    []executil.MockCall
}

func (r *vmRecordingRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, executil.MockCall{Name: name, Args: append([]string(nil), args...)})
	return r.delegate.Run(name, args...)
}

func vmRunnerCalled(runner *vmRecordingRunner, operation string) bool {
	for _, call := range runner.calls {
		if call.Name == "busctl" && len(call.Args) > 4 && call.Args[4] == operation {
			return true
		}
	}
	return false
}
