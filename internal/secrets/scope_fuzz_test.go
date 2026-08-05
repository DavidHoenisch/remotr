package secrets

import (
	"strings"
	"testing"
)

func FuzzParseScope(f *testing.F) {
	for _, seed := range []struct {
		scope, fleet, endpoint string
	}{
		{scope: "global"},
		{scope: "fleet", fleet: "engineering"},
		{scope: "endpoint", endpoint: "endpoint-1"},
		{},
		{scope: "global", fleet: "engineering"},
		{scope: "organization"},
		{scope: " global"},
	} {
		f.Add(seed.scope, seed.fleet, seed.endpoint)
	}

	f.Fuzz(func(t *testing.T, rawScope, fleet, endpointID string) {
		if len(rawScope)+len(fleet)+len(endpointID) > 1024 {
			// test-exception: EXC-041
			t.Skip()
		}
		got, err := ParseScope(rawScope, fleet, endpointID)
		validIdentifier := func(value string) bool {
			return value != "" && value == strings.TrimSpace(value) && len(value) <= 256 && !strings.ContainsAny(value, "\x00\r\n")
		}
		wantValid := false
		switch rawScope {
		case "global":
			wantValid = fleet == "" && endpointID == ""
		case "fleet":
			wantValid = validIdentifier(fleet) && endpointID == ""
		case "endpoint":
			wantValid = validIdentifier(endpointID) && fleet == ""
		}
		if wantValid != (err == nil) {
			t.Fatalf("ParseScope(%q, %q, %q) = %q, %v; valid=%t", rawScope, fleet, endpointID, got, err, wantValid)
		}
		if err == nil && string(got) != rawScope {
			t.Fatalf("successful parse changed scope %q to %q", rawScope, got)
		}
		if rawScope == "" && got == ScopeGlobal {
			t.Fatal("omitted scope became global")
		}
	})
}
