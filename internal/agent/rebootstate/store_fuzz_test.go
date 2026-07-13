package rebootstate

import "testing"

func FuzzParseState(f *testing.F) {
	f.Add([]byte(`{"schemaVersion":1,"required":true,"sources":[{"address":"base/packages/kernel","provider":"apt"}]}`))
	f.Add([]byte(`{"schemaVersion":1,"required":false}`))
	f.Add([]byte(`{"schemaVersion":99,"required":true,"sources":[]}`))
	f.Add([]byte(`not-json`))

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 1<<20 {
			return
		}
		status, err := parseState(raw)
		if err != nil {
			return
		}
		if status.Required != (len(status.Sources) > 0) {
			t.Fatalf("parsed inconsistent reboot state: %+v", status)
		}
	})
}
