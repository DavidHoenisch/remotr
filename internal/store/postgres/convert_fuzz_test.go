package postgres

import (
	"strings"
	"testing"
)

func FuzzUUIDFromString(f *testing.F) {
	f.Add("11111111-1111-1111-1111-111111111111")
	f.Add("")
	f.Add("not-a-uuid")
	f.Add("00000000-0000-0000-0000-000000000000")

	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 512 {
			return
		}
		u, err := uuidFromString(s)
		if err != nil {
			return
		}
		back, err := uuidString(u)
		if err != nil {
			t.Fatal(err)
		}
		if strings.ToLower(back) != strings.ToLower(s) {
			// uuid.Parse normalizes format; only check round-trip consistency
			u2, err2 := uuidFromString(back)
			if err2 != nil {
				t.Fatal(err2)
			}
			back2, err := uuidString(u2)
			if err != nil || back2 != back {
				t.Fatalf("round trip failed for %q", s)
			}
		}
	})
}
