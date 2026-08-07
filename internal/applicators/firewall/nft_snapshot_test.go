package firewall

import (
	"strings"
	"testing"
)

func TestOmitForeignOwnedNFTTablesRemovesFirewalldOwnedTables(t *testing.T) {
	input := strings.Join([]string{
		`table inet firewalld { # progname firewalld`,
		`	chain filter_INPUT {`,
		`		accept`,
		`	}`,
		`}`,
		`table inet remotr_vm_safety {`,
		`	chain input {`,
		`		type filter hook input priority filter; policy accept;`,
		`	}`,
		`}`,
		`table ip nat { # progname something-else`,
		`	chain prerouting {`,
		`	}`,
		`}`,
		``,
	}, "\n")
	got := string(OmitForeignOwnedNFTTables([]byte(input)))
	if strings.Contains(got, "firewalld") || strings.Contains(got, "progname") || strings.Contains(got, "table ip nat") {
		t.Fatalf("foreign-owned tables remained: %q", got)
	}
	if !strings.Contains(got, "table inet remotr_vm_safety {") || !strings.Contains(got, "policy accept;") {
		t.Fatalf("managed remotr table was removed: %q", got)
	}
}

func TestOmitForeignOwnedNFTTablesKeepsUnownedTables(t *testing.T) {
	input := []byte("table inet filter {\n\tchain input {\n\t}\n}\n")
	got := string(OmitForeignOwnedNFTTables(input))
	if got != string(input) {
		t.Fatalf("unowned table rewritten: %q", got)
	}
}
