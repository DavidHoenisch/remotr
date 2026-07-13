package models

import (
	"strings"
	"testing"
)

func TestParseStateCanonicalHostsEntry(t *testing.T) {
	state, err := ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: hostsEntry
        name: api
        address: 203.0.113.10
        canonicalHost: api.example
        aliases: [api.internal]
`))
	if err != nil {
		t.Fatal(err)
	}
	entry := state.Configurations[0].HostsEntries[0]
	if entry.Lifecycle != LifecyclePresent || entry.Ownership != OwnershipNamed || entry.Address != "203.0.113.10" || entry.CanonicalHost != "api.example" || len(entry.Aliases) != 1 {
		t.Fatalf("hosts entry lost canonical fields or defaults: %#v", entry)
	}
}

func TestParseStateRejectsInvalidHostsEntries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fields string
	}{
		{name: "invalid address", fields: "address: not-an-ip\n        canonicalHost: api.example"},
		{name: "duplicate alias", fields: "address: 203.0.113.10\n        canonicalHost: api.example\n        aliases: [API.EXAMPLE]"},
		{name: "absent retains address", fields: "lifecycle: absent\n        address: 203.0.113.10"},
		{name: "unbounded ownership", fields: "ownership: authoritative\n        address: 203.0.113.10\n        canonicalHost: api.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseState(strings.NewReader("schemaVersion: 1\nconfigurations:\n  - name: base\n    resources:\n      - kind: hostsEntry\n        name: api\n        " + tc.fields + "\n"))
			if err == nil {
				t.Fatal("invalid hosts entry was accepted")
			}
		})
	}
}
