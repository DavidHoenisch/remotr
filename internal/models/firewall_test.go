package models

import (
	"strings"
	"testing"
)

func TestParseStateCanonicalAuthoritativeFirewallSet(t *testing.T) {
	state, err := ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: edge
    resources:
      - kind: firewall
        name: web-ingress
        ownership: authoritative
        backend: nftables
        table: filter
        chain: input
        cleanupLimit: 10
        rules:
          - name: https
            action: allow
            protocol: tcp
            ports: [443]
`))
	if err != nil {
		t.Fatal(err)
	}
	r := state.Configurations[0].Firewall[0]
	if r.Ownership != OwnershipAuthoritative || r.CleanupLimit != 10 || len(r.Rules) != 1 || r.Rules[0].Name != "https" {
		t.Fatalf("authoritative firewall lost contract fields: %#v", r)
	}
}

func TestParseStateRejectsUnboundedAuthoritativeFirewallCleanup(t *testing.T) {
	_, err := ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: edge
    resources:
      - kind: firewall
        name: web-ingress
        ownership: authoritative
        backend: nftables
        rules:
          - name: https
            action: allow
            ports: [443]
`))
	if err == nil {
		t.Fatal("authoritative cleanup without a bound must be rejected")
	}
}
