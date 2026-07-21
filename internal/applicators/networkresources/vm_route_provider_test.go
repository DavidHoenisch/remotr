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
	vmRouteConnection = "remotr-route-qualification"
	vmRouteInterface  = "remotr-dns0"
)

func TestRouteProviderVM(t *testing.T) {
	if os.Geteuid() != 0 {
		// test-exception: EXC-039
		t.Skip("route VM contract requires root")
	}
	assertUbuntu2404(t)
	runner := &vmRecordingRunner{delegate: executil.SanitizedOSRunner{}}
	cleanupVMRouteDevice(runner)
	t.Cleanup(func() { cleanupVMRouteDevice(runner) })
	vmRun(t, runner, "ip", "link", "add", vmRouteInterface, "type", "dummy")
	vmRun(t, runner, "nmcli", "connection", "add", "type", "dummy", "ifname", vmRouteInterface,
		"con-name", vmRouteConnection, "ipv4.method", "manual", "ipv4.addresses", "192.0.2.2/24",
		"ipv4.ignore-auto-dns", "yes", "ipv6.method", "disabled")
	vmRun(t, runner, "nmcli", "device", "set", vmRouteInterface, "managed", "yes")
	vmRun(t, runner, "nmcli", "connection", "up", vmRouteConnection)
	originalAddressing := strings.TrimSpace(string(vmRun(t, runner, "nmcli", "-g", "ipv4.method,ipv4.addresses", "connection", "show", vmRouteConnection)))
	controlGateway := vmDefaultGateway(t, runner)

	now := time.Date(2026, 7, 20, 19, 0, 0, 0, time.UTC)
	stateDir := t.TempDir()
	authorized := true
	provider := vmRouteProvider(t, runner, stateDir, fmt.Sprintf("https://%s:8443/v1/sync", controlGateway), func() time.Time { return now }, models.RouteResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Enforce: &authorized},
		Name:         "vm-route", Provider: models.NetworkProviderNetworkManager, Interface: vmRouteInterface,
		Destination: "198.18.0.0/24", Gateway: "192.0.2.1", Metric: 77, Table: 100, Configured: true, Effective: true,
	})
	if check := provider.Check(context.Background()); check.Status != executor.Drifted {
		t.Fatalf("initial route Check = %+v, want drifted", check)
	}
	if err := provider.Preflight(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := provider.PreflightRollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional {
		t.Fatalf("route ApplyResult = %+v, want changed transactional", result)
	}
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("post-Apply route Check = %+v", check)
	}
	assertVMRouteAddressingUnchanged(t, runner, originalAddressing)

	store := vmNetworkStore(t, stateDir, runner, provider.Now)
	status, err := store.Status()
	if err != nil || status.Intent == nil || status.Intent.Phase != networkstate.PhaseAwaitingAcknowledgement {
		t.Fatalf("armed route transaction = %+v, %v", status, err)
	}
	now = now.Add(3 * time.Minute)
	status, err = store.Reconcile(context.Background())
	if err != nil || status.Intent == nil || status.Intent.Phase != networkstate.PhaseRolledBack {
		t.Fatalf("timed-out route transaction = %+v, %v", status, err)
	}
	if !vmRunnerCalled(runner, "CheckpointRollback") {
		t.Fatal("real NetworkManager route checkpoint did not receive CheckpointRollback")
	}
	if check := provider.Check(context.Background()); check.Status != executor.Drifted {
		t.Fatalf("rolled-back route Check = %+v, want prior-state drift", check)
	}
	assertVMRouteAddressingUnchanged(t, runner, originalAddressing)

	result = provider.ApplyResult(context.Background())
	if result.Status != executor.Changed || result.RollbackClass != providercontract.RollbackTransactional {
		t.Fatalf("second route ApplyResult = %+v", result)
	}
	status, err = store.Status()
	if err != nil || status.Intent == nil {
		t.Fatalf("second armed route transaction = %+v, %v", status, err)
	}
	if _, err := store.Acknowledge(context.Background(), status.Intent.ID); err != nil {
		t.Fatal(err)
	}
	if !vmRunnerCalled(runner, "CheckpointDestroy") {
		t.Fatal("authenticated route acknowledgement did not receive CheckpointDestroy")
	}
	secondCheck := provider.Check(context.Background())
	report, ok := secondCheck.Actual.(networkresources.RouteStateReport)
	if secondCheck.Status != executor.Compliant || !ok || !report.Acknowledged || report.RollbackOutcome != "acknowledged" {
		t.Fatalf("second Check = %+v", secondCheck)
	}
	if result := provider.ApplyResult(context.Background()); result.Status != executor.NoChange || result.RollbackClass != executor.RollbackTransactional {
		t.Fatalf("compliant route ApplyResult = %+v", result)
	}
	assertVMRouteAddressingUnchanged(t, runner, originalAddressing)
}

func vmRouteProvider(t *testing.T, runner executil.Runner, stateDir, syncURL string, now func() time.Time, resource models.RouteResource) *networkresources.RouteApplicator {
	t.Helper()
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	resources, err := registry.Resources(&models.Configuration{Routes: []models.RouteResource{resource}})
	if err != nil || len(resources) != 1 || resources[0].Kind() != models.ResourceKindRoute {
		t.Fatalf("route registry resources = %+v, %v", resources, err)
	}
	handler, err := resources[0].NewProvider(resourceregistry.FactoryContext{
		Facts: facts.Facts{Network: facts.NetworkManager}, Runner: runner, StateDir: stateDir, SyncURL: syncURL,
	})
	provider, ok := handler.(*networkresources.RouteApplicator)
	if err != nil || !ok {
		t.Fatalf("route registry provider = %#v, %v", handler, err)
	}
	provider.Now = now
	provider.AfterFunc = func(time.Duration, func()) {}
	return provider
}

func cleanupVMRouteDevice(runner executil.Runner) {
	_, _, _ = runner.Run("nmcli", "connection", "delete", vmRouteConnection)
	_, _, _ = runner.Run("ip", "link", "del", vmRouteInterface)
}

func assertVMRouteAddressingUnchanged(t *testing.T, runner executil.Runner, want string) {
	t.Helper()
	got := strings.TrimSpace(string(vmRun(t, runner, "nmcli", "-g", "ipv4.method,ipv4.addresses", "connection", "show", vmRouteConnection)))
	if got != want {
		t.Fatalf("route-only provider changed addressing: got %q, want %q", got, want)
	}
}
