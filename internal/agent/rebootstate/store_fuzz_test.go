package rebootstate

import "testing"

func FuzzParseState(f *testing.F) {
	f.Add([]byte(`{"schemaVersion":1,"required":true,"sources":[{"address":"base/packages/kernel","provider":"apt"}]}`))
	f.Add([]byte(`{"schemaVersion":1,"required":false}`))
	f.Add([]byte(`{"schemaVersion":99,"required":true,"sources":[]}`))
	f.Add([]byte(`{"schemaVersion":2,"required":false,"attemptGeneration":1,"intent":{"generation":"g1","phase":"attempting","priorBootId":"boot-1","currentBootId":"boot-1","preparedAt":"2026-07-13T02:00:00Z","notBefore":"2026-07-13T02:00:00Z","timeout":60000000000,"attemptedAt":"2026-07-13T02:00:00Z","attemptDeadline":"2026-07-13T02:01:00Z","attemptGeneration":1,"reason":"boot_id_unchanged"}}`))
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
		if status.Intent != nil && status.Intent.AttemptGeneration > status.AttemptGeneration {
			t.Fatalf("intent exceeds durable generation: %+v", status)
		}
		if status.Completion != nil && status.Completion.AttemptGeneration > status.AttemptGeneration {
			t.Fatalf("completion exceeds durable generation: %+v", status)
		}
	})
}
