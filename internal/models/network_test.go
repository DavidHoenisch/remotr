package models

import (
	"strings"
	"testing"
)

func TestParseStateKeepsDNSAndRouteScopesSeparate(t *testing.T) {
	state, err := ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: office
    resources:
      - kind: dnsResolver
        name: corporate-dns
        provider: network-manager
        interface: eth0
        servers: [192.0.2.53, 2001:db8::53]
        searchDomains: [corp.example]
        configured: true
        effective: true
      - kind: route
        name: private-network
        provider: network-manager
        interface: eth0
        destination: 10.20.0.0/16
        gateway: 192.0.2.1
        metric: 50
        configured: true
        effective: true
`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := state.Configurations[0]
	if len(cfg.DNSResolvers) != 1 || len(cfg.Routes) != 1 {
		t.Fatalf("separate network resources were not retained: %#v", cfg)
	}
	dns := cfg.DNSResolvers[0]
	if dns.Provider != NetworkProviderNetworkManager || dns.Interface != "eth0" || !dns.Configured || !dns.Effective || len(dns.Servers) != 2 || len(dns.SearchDomains) != 1 {
		t.Fatalf("DNS resolver fields were lost: %#v", dns)
	}
	route := cfg.Routes[0]
	if route.Provider != NetworkProviderNetworkManager || route.Interface != "eth0" || route.Destination != "10.20.0.0/16" || route.Gateway != "192.0.2.1" || route.Metric != 50 || !route.Configured || !route.Effective {
		t.Fatalf("route fields were lost: %#v", route)
	}
}

func TestParseStateRejectsInvalidNetworkContracts(t *testing.T) {
	for _, tc := range []struct {
		name, resource string
	}{
		{"DNS without managed scope", "kind: dnsResolver\n        name: dns\n        provider: network-manager\n        interface: eth0\n        servers: [192.0.2.53]"},
		{"DNS with oversized label", "kind: dnsResolver\n        name: dns\n        provider: network-manager\n        interface: eth0\n        searchDomains: [aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.example]\n        configured: true"},
		{"route host bits set", "kind: route\n        name: route\n        provider: network-manager\n        interface: eth0\n        destination: 10.20.1.1/16\n        configured: true"},
		{"route family mismatch", "kind: route\n        name: route\n        provider: network-manager\n        interface: eth0\n        destination: 10.20.0.0/16\n        gateway: 2001:db8::1\n        effective: true"},
		{"route metric out of range", "kind: route\n        name: route\n        provider: network-manager\n        interface: eth0\n        destination: 10.20.0.0/16\n        metric: 4294967296\n        configured: true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseState(strings.NewReader("schemaVersion: 1\nconfigurations:\n  - name: office\n    resources:\n      - " + tc.resource + "\n"))
			if err == nil {
				t.Fatal("invalid network contract was accepted")
			}
		})
	}
}

func FuzzParseCanonicalNetworkResource(f *testing.F) {
	for _, seed := range []string{
		"kind: dnsResolver\nname: dns\nprovider: network-manager\ninterface: eth0\nservers: [192.0.2.53]\nconfigured: true",
		"kind: route\nname: route\nprovider: network-manager\ninterface: eth0\ndestination: 10.20.0.0/16\neffective: true",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, resource string) {
		if len(resource) > 4096 {
			t.Skip()
		}
		_, _ = ParseState(strings.NewReader("schemaVersion: 1\nconfigurations:\n  - name: fuzz\n    resources:\n      - " + strings.ReplaceAll(resource, "\n", "\n        ") + "\n"))
	})
}
