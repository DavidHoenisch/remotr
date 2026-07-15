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
		"kind: networkProfile\nname: profile\nprovider: network-manager\nselector: {name: eth0}\nprofileName: office\nprofileType: ethernet",
		"kind: networkProfile\nname: profile\nprovider: netplan\nselector: {name: eth0}\nprofileName: office\nprofileType: ethernet",
		"kind: networkProfile\nname: profile\nprovider: systemd-networkd\nselector: {name: eth0}\nprofileName: office\nprofileType: ethernet",
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

func TestParseStateRejectsInvalidNetworkProfiles(t *testing.T) {
	for _, resource := range []string{
		"selector: {}\n        profileName: office\n        profileType: ethernet",
		"selector: {type: wifi}\n        profileName: office\n        profileType: ethernet",
		"selector: {name: wlan0}\n        profileName: office\n        profileType: wifi\n        ssid: corp\n        credentialRef: inline:secret",
		"selector: {name: eth0}\n        profileName: office\n        profileType: ethernet\n        mtu: 10000",
		"selector: {name: wlan0}\n        profileName: office\n        profileType: wifi",
	} {
		_, err := ParseState(strings.NewReader("schemaVersion: 1\nconfigurations:\n  - name: office\n    resources:\n      - kind: networkProfile\n        name: profile\n        provider: network-manager\n        " + resource + "\n"))
		if err == nil {
			t.Fatalf("invalid network profile was accepted:\n%s", resource)
		}
	}
}

func TestParseStateCanonicalNetworkProfile(t *testing.T) {
	state, err := ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: office
    resources:
      - kind: networkProfile
        name: wifi
        provider: network-manager
        selector:
          name: wlan0
          permanentMAC: 02:00:00:00:00:0a
          type: wifi
        profileName: office
        profileType: wifi
        autoConnect: true
        mtu: 1500
        ipv4Method: auto
        ipv6Method: ignore
        ssid: corp
        credentialRef: remotr:wifi/office
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Configurations[0].NetworkProfiles) != 1 {
		t.Fatalf("network profile was not retained: %#v", state)
	}
	profile := state.Configurations[0].NetworkProfiles[0]
	if !profile.IsAudit() || profile.Provider != NetworkProviderNetworkManager || profile.Selector.Name != "wlan0" || profile.CredentialRef != "remotr:wifi/office" {
		t.Fatalf("network profile fields/defaults were lost: %#v", profile)
	}
}

func TestParseStateBoundsEnforcedNetworkProfileRollbackTimeout(t *testing.T) {
	for _, tc := range []struct {
		name, timeout string
		valid         bool
	}{
		{name: "missing", valid: false},
		{name: "too short", timeout: "29s", valid: false},
		{name: "lower bound", timeout: "30s", valid: true},
		{name: "upper bound", timeout: "15m", valid: true},
		{name: "too long", timeout: "15m1s", valid: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseState(strings.NewReader("schemaVersion: 1\nconfigurations:\n  - name: office\n    resources:\n      - kind: networkProfile\n        name: profile\n        provider: network-manager\n        selector: {name: eth0}\n        profileName: office\n        profileType: ethernet\n        audit: false\n        enforce: true\n        rollbackTimeout: " + tc.timeout + "\n"))
			if tc.valid && err != nil {
				t.Fatal(err)
			}
			if !tc.valid && err == nil {
				t.Fatal("invalid enforced rollback timeout was accepted")
			}
		})
	}
}

func TestParseStateAcceptsSafeFileBackedNetworkProfileProviders(t *testing.T) {
	for _, provider := range []string{"netplan", "systemd-networkd"} {
		t.Run(provider, func(t *testing.T) {
			state, err := ParseState(strings.NewReader("schemaVersion: 1\nconfigurations:\n  - name: office\n    resources:\n      - kind: networkProfile\n        name: uplink\n        provider: " + provider + "\n        selector: {name: eth0, type: ethernet}\n        profileName: office\n        profileType: ethernet\n        ipv4Method: auto\n        audit: false\n        enforce: true\n        rollbackTimeout: 2m\n"))
			if err != nil {
				t.Fatal(err)
			}
			if got := state.Configurations[0].NetworkProfiles[0].Provider; got != provider {
				t.Fatalf("provider = %q", got)
			}
		})
	}
}

func TestParseStateRejectsUnsafeFileBackedProfileCapabilities(t *testing.T) {
	for _, resource := range []string{
		"provider: systemd-networkd\n        selector: {name: wlan0, type: wifi}\n        profileName: office\n        profileType: wifi\n        ssid: corp",
		"provider: netplan\n        selector: {name: wlan0, type: wifi}\n        profileName: office\n        profileType: wifi\n        ssid: corp\n        credentialRef: remotr:wifi/office\n        audit: false\n        enforce: true\n        rollbackTimeout: 2m",
	} {
		_, err := ParseState(strings.NewReader("schemaVersion: 1\nconfigurations:\n  - name: office\n    resources:\n      - kind: networkProfile\n        name: profile\n        " + resource + "\n"))
		if err == nil {
			t.Fatalf("unsafe file-backed profile was accepted:\n%s", resource)
		}
	}
}
