package postgres

import (
	"testing"
)

func FuzzParseEndpointID(f *testing.F) {
	f.Add("11111111-1111-1111-1111-111111111111")
	f.Add("stllr-remotr-a1b2c3d4")
	f.Add("")
	f.Add("not valid")
	f.Add("00000000-0000-0000-0000-000000000000")

	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 512 {
			return
		}
		got, err := parseEndpointID(s)
		if err != nil {
			return
		}
		got2, err := parseEndpointID(got)
		if err != nil || got2 != got {
			t.Fatalf("round trip failed for %q", s)
		}
	})
}
