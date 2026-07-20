package networkresources

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestDNSApplicatorChangesOnlyConfiguredAndEffectiveResolverState(t *testing.T) {
	checkpoint := "/org/freedesktop/NetworkManager/Checkpoint/11"
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"nmcli [-t -f GENERAL.CONNECTION device show eth0]": {Stdout: []byte("GENERAL.CONNECTION:office\n")},
		"nmcli [-t -f ipv4.dns,ipv6.dns,ipv4.dns-search,ipv6.dns-search connection show office]": {
			Stdout: []byte("ipv4.dns:192.0.2.1\nipv6.dns:\nipv4.dns-search:old.example\nipv6.dns-search:\n"),
		},
		"nmcli [-t -f IP4.DNS,IP6.DNS,IP4.DOMAIN,IP6.DOMAIN device show eth0]": {
			Stdout: []byte("IP4.DNS[1]:192.0.2.1\nIP4.DOMAIN[1]:old.example\n"),
		},
		"nmcli [connection modify office ipv4.ignore-auto-dns yes ipv4.dns 192.0.2.53 ipv6.ignore-auto-dns yes ipv6.dns 2001:db8::53 ipv4.dns-search corp.example ipv6.dns-search corp.example]": {},
		"nmcli [device reapply eth0]":                   {},
		"nmcli [-g GENERAL.DBUS-PATH device show eth0]": {Stdout: []byte("/org/freedesktop/NetworkManager/Devices/2\n")},
		"ip [-json route get 203.0.113.10]":             {Stdout: []byte(`[{"dst":"203.0.113.10","dev":"eth0"}]`)},
		"busctl [call org.freedesktop.NetworkManager /org/freedesktop/NetworkManager org.freedesktop.NetworkManager CheckpointCreate aou 1 /org/freedesktop/NetworkManager/Devices/2 120 0]": {Stdout: []byte("o \"" + checkpoint + "\"\n")},
	}}
	authorized := true
	resource := models.DNSResolverResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Enforce: &authorized},
		Name:         "corporate-dns", Provider: models.NetworkProviderNetworkManager, Interface: "eth0",
		Servers: []string{"192.0.2.53", "2001:db8::53"}, SearchDomains: []string{"corp.example"}, Configured: true, Effective: true,
	}
	provider := NewDNS(resource, runner)
	configureDNSTestSafety(t, provider)
	check := provider.Check(context.Background())
	if check.Status != executor.Drifted {
		t.Fatalf("Check() = %+v", check)
	}
	report, ok := check.Actual.(DNSStateReport)
	if !ok || report.Configured.Compliant || report.Effective.Compliant || report.Backend != models.NetworkProviderNetworkManager {
		t.Fatalf("configured/effective report = %#v", check.Actual)
	}
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, call := range runner.Calls {
		joined := call.Name + " " + strings.Join(call.Args, " ")
		if (strings.HasPrefix(joined, "ip ") && !strings.HasPrefix(joined, "ip -json route get ")) || strings.Contains(joined, "addresses") || strings.Contains(joined, "routes") {
			t.Fatalf("DNS-only apply touched route or addressing state: %s", joined)
		}
	}
}

func TestDNSApplicatorActivatesConfiguredStateThroughNetworkManager(t *testing.T) {
	checkpoint := "/org/freedesktop/NetworkManager/Checkpoint/12"
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"nmcli [-t -f GENERAL.CONNECTION device show eth0]": {Stdout: []byte("GENERAL.CONNECTION:office\n")},
		"nmcli [-t -f ipv4.dns,ipv6.dns,ipv4.dns-search,ipv6.dns-search connection show office]": {
			Stdout: []byte("ipv4.dns:192.0.2.1\nipv4.dns-search:old.example\n"),
		},
		"nmcli [-t -f IP4.DNS,IP6.DNS,IP4.DOMAIN,IP6.DOMAIN device show eth0]": {
			Stdout: []byte("IP4.DNS[1]:192.0.2.1\nIP4.DOMAIN[1]:old.example\n"),
		},
		"nmcli [connection modify office ipv4.ignore-auto-dns yes ipv4.dns 192.0.2.53 ipv6.ignore-auto-dns yes ipv6.dns  ipv4.dns-search corp.example ipv6.dns-search corp.example]": {},
		"nmcli [device reapply eth0]":                   {},
		"nmcli [-g GENERAL.DBUS-PATH device show eth0]": {Stdout: []byte("/org/freedesktop/NetworkManager/Devices/2\n")},
		"ip [-json route get 203.0.113.10]":             {Stdout: []byte(`[{"dst":"203.0.113.10","dev":"eth0"}]`)},
		"busctl [call org.freedesktop.NetworkManager /org/freedesktop/NetworkManager org.freedesktop.NetworkManager CheckpointCreate aou 1 /org/freedesktop/NetworkManager/Devices/2 120 0]": {Stdout: []byte("o \"" + checkpoint + "\"\n")},
	}}
	authorized := true
	provider := NewDNS(models.DNSResolverResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Enforce: &authorized},
		Name:         "corporate-dns", Provider: models.NetworkProviderNetworkManager, Interface: "eth0",
		Servers: []string{"192.0.2.53"}, SearchDomains: []string{"corp.example"}, Configured: true, Effective: true,
	}, runner)
	configureDNSTestSafety(t, provider)

	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	foundReapply := false
	for _, call := range runner.Calls {
		if call.Name == "resolvectl" {
			t.Fatalf("NetworkManager DNS provider crossed into resolvectl: %+v", runner.Calls)
		}
		if call.Name == "nmcli" && strings.Join(call.Args, " ") == "device reapply eth0" {
			foundReapply = true
		}
	}
	if !foundReapply {
		t.Fatalf("NetworkManager DNS activation calls = %+v, want device reapply", runner.Calls)
	}
}

func configureDNSTestSafety(t *testing.T, provider *DNSApplicator) {
	t.Helper()
	provider.StateDir = t.TempDir()
	provider.SyncURL = "https://203.0.113.10:8443/v1/sync"
	provider.Now = func() time.Time { return time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC) }
	provider.AfterFunc = func(time.Duration, func()) {}
}

func TestDNSApplicatorArmsCheckpointBeforeMutationAndRollsBackWithoutAcknowledgement(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	checkpoint := "/org/freedesktop/NetworkManager/Checkpoint/61"
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"nmcli [-t -f GENERAL.CONNECTION device show eth0]": {Stdout: []byte("GENERAL.CONNECTION:office\n")},
		"nmcli [-t -f ipv4.dns,ipv6.dns,ipv4.dns-search,ipv6.dns-search connection show office]": {
			Stdout: []byte("ipv4.dns:192.0.2.1\nipv4.dns-search:old.example\n"),
		},
		"nmcli [-t -f IP4.DNS,IP6.DNS,IP4.DOMAIN,IP6.DOMAIN device show eth0]": {
			Stdout: []byte("IP4.DNS[1]:192.0.2.1\nIP4.DOMAIN[1]:old.example\n"),
		},
		"nmcli [-g GENERAL.DBUS-PATH device show eth0]": {
			Stdout: []byte("/org/freedesktop/NetworkManager/Devices/2\n"),
		},
		"ip [-json route get 203.0.113.10]": {
			Stdout: []byte(`[{"dst":"203.0.113.10","gateway":"192.0.2.1","dev":"eth0"}]`),
		},
		"busctl [call org.freedesktop.NetworkManager /org/freedesktop/NetworkManager org.freedesktop.NetworkManager CheckpointCreate aou 1 /org/freedesktop/NetworkManager/Devices/2 120 0]": {
			Stdout: []byte("o \"" + checkpoint + "\"\n"),
		},
		"nmcli [connection modify office ipv4.ignore-auto-dns yes ipv4.dns 192.0.2.53 ipv6.ignore-auto-dns yes ipv6.dns  ipv4.dns-search corp.example ipv6.dns-search corp.example]": {},
		"nmcli [device reapply eth0]": {},
		"busctl [call org.freedesktop.NetworkManager /org/freedesktop/NetworkManager org.freedesktop.NetworkManager CheckpointRollback o " + checkpoint + "]": {},
	}}
	authorized := true
	provider := NewDNS(models.DNSResolverResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Enforce: &authorized},
		Name:         "corporate-dns", Provider: models.NetworkProviderNetworkManager, Interface: "eth0",
		Servers: []string{"192.0.2.53"}, SearchDomains: []string{"corp.example"}, Configured: true, Effective: true,
	}, runner)
	provider.StateDir = t.TempDir()
	provider.SyncURL = "https://control.example:8443/v1/sync"
	provider.ResolveIP = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}, nil
	}
	provider.Now = func() time.Time { return now }
	var watchdog func()
	provider.AfterFunc = func(delay time.Duration, callback func()) {
		if delay != 2*time.Minute {
			t.Fatalf("rollback delay = %s, want 2m", delay)
		}
		watchdog = callback
	}

	if err := provider.Preflight(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := provider.PreflightRollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional {
		t.Fatalf("ApplyResult() = %+v, want changed transactional", result)
	}
	checkpointCall, mutationCall := -1, -1
	for i, call := range runner.Calls {
		joined := call.Name + " " + strings.Join(call.Args, " ")
		if strings.Contains(joined, "CheckpointCreate") {
			checkpointCall = i
		}
		if strings.HasPrefix(joined, "nmcli connection modify") {
			mutationCall = i
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
	if err != nil || status.Intent == nil || status.Intent.Phase != networkstate.PhaseAwaitingAcknowledgement || status.Intent.Checkpoint != checkpoint {
		t.Fatalf("armed DNS transaction = %+v, %v", status, err)
	}
	if watchdog == nil {
		t.Fatal("DNS rollback watchdog was not armed")
	}
	now = now.Add(3 * time.Minute)
	watchdog()
	status, err = store.Status()
	if err != nil || status.Intent == nil || status.Intent.Phase != networkstate.PhaseRolledBack {
		t.Fatalf("timed-out DNS transaction = %+v, %v", status, err)
	}
}

func TestDNSApplicatorReportsAuthenticatedAcknowledgement(t *testing.T) {
	now := time.Date(2026, 7, 20, 14, 0, 0, 0, time.UTC)
	checkpoint := "/org/freedesktop/NetworkManager/Checkpoint/62"
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"nmcli [-t -f GENERAL.CONNECTION device show eth0]": {Stdout: []byte("GENERAL.CONNECTION:office\n")},
		"nmcli [-t -f ipv4.dns,ipv6.dns,ipv4.dns-search,ipv6.dns-search connection show office]": {
			Stdout: []byte("ipv4.dns:192.0.2.53\nipv4.dns-search:corp.example\n"),
		},
		"nmcli [-t -f IP4.DNS,IP6.DNS,IP4.DOMAIN,IP6.DOMAIN device show eth0]": {
			Stdout: []byte("IP4.DNS[1]:192.0.2.53\nIP4.DOMAIN[1]:corp.example\n"),
		},
		"busctl [call org.freedesktop.NetworkManager /org/freedesktop/NetworkManager org.freedesktop.NetworkManager CheckpointDestroy o " + checkpoint + "]": {},
	}}
	authorized := true
	provider := NewDNS(models.DNSResolverResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Enforce: &authorized},
		Name:         "corporate-dns", Provider: models.NetworkProviderNetworkManager, Interface: "eth0",
		Servers: []string{"192.0.2.53"}, SearchDomains: []string{"corp.example"}, Configured: true, Effective: true,
	}, runner)
	provider.StateDir = t.TempDir()
	provider.Now = func() time.Time { return now }
	store, err := networkstate.New(networkstate.Options{Root: provider.StateDir, Runner: runner, Now: provider.Now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Prepare(context.Background(), networkstate.Intent{
		ID: "dns-acknowledgement", Address: "dnsResolver/corporate-dns", ArtifactDigest: "sha256:dns", Attempt: 1,
		Backend: "network-manager", Deadline: now.Add(2 * time.Minute), Checkpoint: checkpoint,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Acknowledge(context.Background(), "dns-acknowledgement"); err != nil {
		t.Fatal(err)
	}

	check := provider.Check(context.Background())
	if check.Status != executor.Compliant {
		t.Fatalf("Check() = %+v", check)
	}
	raw, err := json.Marshal(check.Actual)
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if report["acknowledged"] != true || report["rollbackOutcome"] != "acknowledged" {
		t.Fatalf("DNS acknowledgement report = %s", raw)
	}
	if strings.Contains(string(raw), checkpoint) || strings.Contains(string(raw), "203.0.113") {
		t.Fatalf("DNS acknowledgement report exposed recovery internals: %s", raw)
	}
}

func TestDNSApplicatorReportsUnadvertisedBackendAsUnsupported(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{}}
	provider := NewDNS(models.DNSResolverResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "corporate-dns", Provider: models.NetworkProviderNetplan, Interface: "eth0",
		Servers: []string{"192.0.2.53"}, Configured: true,
	}, runner)

	check := provider.Check(context.Background())
	if check.Status != executor.Unsupported || check.ReasonCode != executor.ReasonProviderUnavailable {
		t.Fatalf("Check() = %+v, want typed unsupported backend", check)
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("unsupported DNS backend crossed process boundary: %+v", runner.Calls)
	}
}
