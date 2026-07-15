package facts

import "testing"

func TestDetectNetworkBackendPrefersConfigurationOwner(t *testing.T) {
	for _, tc := range []struct {
		name      string
		available map[string]bool
		want      NetworkBackend
	}{
		{name: "NetworkManager owns all", available: map[string]bool{"nmcli": true, "netplan": true, "networkctl": true}, want: NetworkManager},
		{name: "netplan owns renderer", available: map[string]bool{"netplan": true, "networkctl": true}, want: NetworkNetplan},
		{name: "native networkd", available: map[string]bool{"networkctl": true}, want: NetworkSystemdNetwork},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := detectNetworkBackend(func(name string) bool { return tc.available[name] })
			if got != tc.want {
				t.Fatalf("backend = %q, want %q", got, tc.want)
			}
		})
	}
}
