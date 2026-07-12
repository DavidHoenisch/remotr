package models

import (
	"strings"
	"testing"
)

func FuzzResourceAddressRoundTrip(f *testing.F) {
	f.Add("base", "curl")
	f.Add("base-packages", "internal/mycli")

	f.Fuzz(func(t *testing.T, configuration, resource string) {
		if len(configuration) > 256 || len(resource) > 256 || strings.ContainsAny(configuration+resource, "\x00\n\r") {
			return
		}
		address := ResourceAddress(configuration, resource)
		parts := strings.SplitN(address, "/", 2)
		if len(parts) != 2 || parts[0] != configuration || parts[1] != resource {
			t.Fatalf("address %q did not preserve (%q, %q)", address, configuration, resource)
		}
	})
}
