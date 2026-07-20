package networkresources

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestRouteApplicatorReportsEffectiveDriftSeparately(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"nmcli [-t -f GENERAL.CONNECTION device show eth0]":  {Stdout: []byte("GENERAL.CONNECTION:office\n")},
		"nmcli [-g ipv4.routes connection show office]":      {Stdout: []byte("10.20.0.0/16 192.0.2.1 50, table=254\n")},
		"ip [-json route show exact 10.20.0.0/16 table 254]": {Stdout: []byte("[]\n")},
		"nmcli [device reapply eth0]":                        {},
	}}
	resource := models.RouteResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "private-network", Provider: models.NetworkProviderNetworkManager, Interface: "eth0",
		Destination: "10.20.0.0/16", Gateway: "192.0.2.1", Metric: 50, Table: 254, Configured: true, Effective: true,
	}
	provider := NewRoute(resource, runner)
	check := provider.Check(context.Background())
	if check.Status != executor.Drifted {
		t.Fatalf("Check() = %+v", check)
	}
	report, ok := check.Actual.(RouteStateReport)
	if !ok || !report.Configured.Compliant || report.Effective.Compliant {
		t.Fatalf("route scope report = %#v", check.Actual)
	}
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, call := range runner.Calls {
		if call.Name == "nmcli" && len(call.Args) > 1 && call.Args[0] == "connection" && call.Args[1] == "modify" {
			t.Fatalf("runtime-only drift rewrote persistent route: %+v", call)
		}
		if call.Name == "ip" && len(call.Args) > 1 && call.Args[0] == "route" {
			t.Fatalf("NetworkManager route provider crossed into raw route mutation: %+v", call)
		}
	}
}

func TestRouteApplicatorReportsUnadvertisedBackendAsUnsupported(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{}}
	provider := NewRoute(models.RouteResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "private-network", Provider: models.NetworkProviderNetplan, Interface: "eth0",
		Destination: "10.20.0.0/16", Configured: true,
	}, runner)

	check := provider.Check(context.Background())
	if check.Status != executor.Unsupported || check.ReasonCode != executor.ReasonProviderUnavailable {
		t.Fatalf("Check() = %+v, want typed unsupported backend", check)
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("unsupported route backend crossed process boundary: %+v", runner.Calls)
	}
}

func TestRouteApplicatorArmsCheckpointBeforeMutationAndRollsBackWithoutAcknowledgement(t *testing.T) {
	now := time.Date(2026, 7, 20, 18, 0, 0, 0, time.UTC)
	checkpoint := "/org/freedesktop/NetworkManager/Checkpoint/81"
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"nmcli [-t -f GENERAL.CONNECTION device show eth0]":  {Stdout: []byte("GENERAL.CONNECTION:office\n")},
		"nmcli [-g ipv4.routes connection show office]":      {Stdout: []byte("\n")},
		"ip [-json route show exact 10.20.0.0/16 table 254]": {Stdout: []byte("[]\n")},
		"ip [-json route get 203.0.113.10]":                  {Stdout: []byte(`[{"dst":"203.0.113.10","gateway":"192.0.2.1","dev":"eth0"}]`)},
		"nmcli [-g GENERAL.DBUS-PATH device show eth0]":      {Stdout: []byte("/org/freedesktop/NetworkManager/Devices/2\n")},
		"busctl [call org.freedesktop.NetworkManager /org/freedesktop/NetworkManager org.freedesktop.NetworkManager CheckpointCreate aouu 1 /org/freedesktop/NetworkManager/Devices/2 120 0]": {Stdout: []byte("o \"" + checkpoint + "\"\n")},
		"nmcli [connection modify office +ipv4.routes 10.20.0.0/16 192.0.2.1 50, table=254]":                                                                                                  {},
		"nmcli [device reapply eth0]": {},
		"busctl [call org.freedesktop.NetworkManager /org/freedesktop/NetworkManager org.freedesktop.NetworkManager CheckpointRollback o " + checkpoint + "]": {},
		"nmcli [-w 30 connection up office ifname eth0]": {},
	}}
	authorized := true
	provider := NewRoute(models.RouteResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Enforce: &authorized},
		Name:         "private-network", Provider: models.NetworkProviderNetworkManager, Interface: "eth0",
		Destination: "10.20.0.0/16", Gateway: "192.0.2.1", Metric: 50, Table: 254, Configured: true, Effective: true,
	}, runner)
	provider.StateDir = t.TempDir()
	provider.SyncURL = "https://control.example:8443/v1/sync"
	provider.ResolveIP = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}, nil
	}
	provider.Now = func() time.Time { return now }
	provider.AfterFunc = func(time.Duration, func()) {}

	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional {
		t.Fatalf("ApplyResult() = %+v, want changed transactional", result)
	}
	checkpointCall, mutationCall := -1, -1
	for index, call := range runner.Calls {
		joined := call.Name + " " + strings.Join(call.Args, " ")
		if strings.Contains(joined, "CheckpointCreate") {
			checkpointCall = index
		}
		if strings.HasPrefix(joined, "nmcli connection modify") {
			mutationCall = index
		}
	}
	if checkpointCall < 0 || mutationCall < 0 || checkpointCall >= mutationCall {
		t.Fatalf("checkpoint/mutation order = %d/%d in %+v", checkpointCall, mutationCall, runner.Calls)
	}
	store, err := networkstate.New(networkstate.Options{Root: provider.StateDir, Runner: runner, Now: provider.Now})
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.Status()
	if err != nil || status.Intent == nil || status.Intent.Phase != networkstate.PhaseAwaitingAcknowledgement ||
		status.Intent.Address != "route/private-network" || status.Intent.Interface != "eth0" || status.Intent.Connection != "office" {
		t.Fatalf("armed route transaction = %+v, %v", status, err)
	}
	now = now.Add(3 * time.Minute)
	status, err = store.Reconcile(context.Background())
	if err != nil || status.Intent == nil || status.Intent.Phase != networkstate.PhaseRolledBack {
		t.Fatalf("timed-out route transaction = %+v, %v", status, err)
	}
}
