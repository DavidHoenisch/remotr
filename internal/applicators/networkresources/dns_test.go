package networkresources

import (
	"context"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestDNSApplicatorChangesOnlyConfiguredAndEffectiveResolverState(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"nmcli [-t -f GENERAL.CONNECTION device show eth0]": {Stdout: []byte("GENERAL.CONNECTION:office\n")},
		"nmcli [-t -f ipv4.dns,ipv6.dns,ipv4.dns-search,ipv6.dns-search connection show office]": {
			Stdout: []byte("ipv4.dns:192.0.2.1\nipv6.dns:\nipv4.dns-search:old.example\nipv6.dns-search:\n"),
		},
		"nmcli [-t -f IP4.DNS,IP6.DNS,IP4.DOMAIN,IP6.DOMAIN device show eth0]": {
			Stdout: []byte("IP4.DNS[1]:192.0.2.1\nIP4.DOMAIN[1]:old.example\n"),
		},
		"nmcli [connection modify office ipv4.ignore-auto-dns yes ipv4.dns 192.0.2.53 ipv6.ignore-auto-dns yes ipv6.dns 2001:db8::53 ipv4.dns-search corp.example ipv6.dns-search corp.example]": {},
		"resolvectl [dns eth0 192.0.2.53 2001:db8::53]": {},
		"resolvectl [domain eth0 corp.example]":         {},
	}}
	resource := models.DNSResolverResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "corporate-dns", Provider: models.NetworkProviderNetworkManager, Interface: "eth0",
		Servers: []string{"192.0.2.53", "2001:db8::53"}, SearchDomains: []string{"corp.example"}, Configured: true, Effective: true,
	}
	provider := NewDNS(resource, runner)
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
		if strings.HasPrefix(joined, "ip ") || strings.Contains(joined, "addresses") || strings.Contains(joined, "routes") {
			t.Fatalf("DNS-only apply touched route or addressing state: %s", joined)
		}
	}
}

func TestDNSApplicatorActivatesConfiguredStateThroughNetworkManager(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"nmcli [-t -f GENERAL.CONNECTION device show eth0]": {Stdout: []byte("GENERAL.CONNECTION:office\n")},
		"nmcli [-t -f ipv4.dns,ipv6.dns,ipv4.dns-search,ipv6.dns-search connection show office]": {
			Stdout: []byte("ipv4.dns:192.0.2.1\nipv4.dns-search:old.example\n"),
		},
		"nmcli [-t -f IP4.DNS,IP6.DNS,IP4.DOMAIN,IP6.DOMAIN device show eth0]": {
			Stdout: []byte("IP4.DNS[1]:192.0.2.1\nIP4.DOMAIN[1]:old.example\n"),
		},
		"nmcli [connection modify office ipv4.ignore-auto-dns yes ipv4.dns 192.0.2.53 ipv6.ignore-auto-dns yes ipv6.dns  ipv4.dns-search corp.example ipv6.dns-search corp.example]": {},
		"nmcli [device reapply eth0]": {},
	}}
	provider := NewDNS(models.DNSResolverResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "corporate-dns", Provider: models.NetworkProviderNetworkManager, Interface: "eth0",
		Servers: []string{"192.0.2.53"}, SearchDomains: []string{"corp.example"}, Configured: true, Effective: true,
	}, runner)

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
