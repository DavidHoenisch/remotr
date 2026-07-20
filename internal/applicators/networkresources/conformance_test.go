package networkresources_test

import (
	"context"
	"fmt"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/applicators/networkresources"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	harness "github.com/DavidHoenisch/remotr/test/providercontract"
)

func TestDNSApplicatorConvergesConfiguredAndEffectiveScopes(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider { return dnsContractProvider(t, true) },
		Drifted:   func(t *testing.T) contract.Provider { return dnsContractProvider(t, false) },
	})
}

func TestRouteApplicatorConvergesConfiguredAndEffectiveScopes(t *testing.T) {
	harness.RunConvergence(t, harness.Fixture{
		Compliant: func(t *testing.T) contract.Provider { return routeContractProvider(t, true) },
		Drifted:   func(t *testing.T) contract.Provider { return routeContractProvider(t, false) },
	})
}

func dnsContractProvider(t *testing.T, compliant bool) contract.Provider {
	t.Helper()
	runner := &dnsRunner{configured: compliant, effective: compliant}
	authorized := true
	providerApplicator := networkresources.NewDNS(models.DNSResolverResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent, Enforce: &authorized},
		Name:         "dns", Provider: models.NetworkProviderNetworkManager, Interface: "eth0",
		Servers: []string{"192.0.2.53"}, SearchDomains: []string{"corp.example"}, Configured: true, Effective: true,
	}, runner)
	providerApplicator.StateDir = t.TempDir()
	providerApplicator.SyncURL = "https://control.example:8443/v1/sync"
	providerApplicator.ResolveIP = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}, nil
	}
	providerApplicator.Now = func() time.Time { return time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC) }
	providerApplicator.AfterFunc = func(time.Duration, func()) {}
	provider, err := contract.New(providerApplicator)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

type dnsRunner struct{ configured, effective bool }

func (r *dnsRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	if name == "nmcli" && slices.Equal(args, []string{"-t", "-f", "GENERAL.CONNECTION", "device", "show", "eth0"}) {
		return []byte("GENERAL.CONNECTION:office\n"), nil, nil
	}
	if name == "nmcli" && slices.Equal(args, []string{"-t", "-f", "ipv4.dns,ipv6.dns,ipv4.dns-search,ipv6.dns-search", "connection", "show", "office"}) {
		if r.configured {
			return []byte("ipv4.dns:192.0.2.53\nipv6.dns:\nipv4.dns-search:corp.example\nipv6.dns-search:corp.example\n"), nil, nil
		}
		return []byte("ipv4.dns:192.0.2.1\nipv4.dns-search:old.example\n"), nil, nil
	}
	if name == "nmcli" && slices.Equal(args, []string{"-t", "-f", "IP4.DNS,IP6.DNS,IP4.SEARCHES,IP6.SEARCHES", "device", "show", "eth0"}) {
		if r.effective {
			return []byte("IP4.DNS[1]:192.0.2.53\nIP4.SEARCHES[1]:corp.example\n"), nil, nil
		}
		return []byte("IP4.DNS[1]:192.0.2.1\nIP4.SEARCHES[1]:old.example\n"), nil, nil
	}
	if name == "nmcli" && len(args) > 2 && args[0] == "connection" && args[1] == "modify" {
		r.configured = true
		return nil, nil, nil
	}
	if name == "nmcli" && slices.Equal(args, []string{"device", "reapply", "eth0"}) {
		r.effective = true
		return nil, nil, nil
	}
	if name == "nmcli" && slices.Equal(args, []string{"-g", "GENERAL.DBUS-PATH", "device", "show", "eth0"}) {
		return []byte("/org/freedesktop/NetworkManager/Devices/2\n"), nil, nil
	}
	if name == "ip" && slices.Equal(args, []string{"-json", "route", "get", "203.0.113.10"}) {
		return []byte(`[{"dst":"203.0.113.10","dev":"eth0"}]`), nil, nil
	}
	if name == "busctl" && len(args) > 4 && args[4] == "CheckpointCreate" {
		return []byte("o \"/org/freedesktop/NetworkManager/Checkpoint/71\"\n"), nil, nil
	}
	return nil, nil, fmt.Errorf("unexpected DNS command %s %v", name, args)
}

func routeContractProvider(t *testing.T, compliant bool) contract.Provider {
	t.Helper()
	runner := &routeRunner{configured: compliant, effective: compliant}
	provider, err := contract.New(networkresources.NewRoute(models.RouteResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "route", Provider: models.NetworkProviderNetworkManager, Interface: "eth0",
		Destination: "10.20.0.0/16", Gateway: "192.0.2.1", Metric: 50, Table: 254, Configured: true, Effective: true,
	}, runner))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

type routeRunner struct{ configured, effective bool }

func (r *routeRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	if name == "nmcli" && slices.Equal(args, []string{"-t", "-f", "GENERAL.CONNECTION", "device", "show", "eth0"}) {
		return []byte("GENERAL.CONNECTION:office\n"), nil, nil
	}
	if name == "nmcli" && slices.Equal(args, []string{"-g", "ipv4.routes", "connection", "show", "office"}) {
		if r.configured {
			return []byte("10.20.0.0/16 192.0.2.1 50, table=254\n"), nil, nil
		}
		return nil, nil, nil
	}
	if name == "ip" && slices.Equal(args, []string{"-json", "route", "show", "exact", "10.20.0.0/16", "table", "254"}) {
		if r.effective {
			return []byte(`[{"dst":"10.20.0.0/16","gateway":"192.0.2.1","dev":"eth0","metric":50,"table":254}]`), nil, nil
		}
		return []byte("[]"), nil, nil
	}
	if name == "nmcli" && len(args) > 2 && args[0] == "connection" && args[1] == "modify" {
		r.configured = true
		return nil, nil, nil
	}
	if name == "ip" && len(args) > 2 && args[0] == "route" && args[1] == "replace" {
		r.effective = true
		return nil, nil, nil
	}
	return nil, nil, fmt.Errorf("unexpected route command %s %v", name, args)
}
