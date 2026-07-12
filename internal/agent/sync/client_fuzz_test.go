package sync

import (
	"encoding/json"
	"testing"
)

func FuzzResponseSupportsMixedVersionPayloads(f *testing.F) {
	f.Add([]byte(`{"unchanged":false,"releaseRef":"r1","digest":"d1","futureSchema":2}`))
	f.Add([]byte(`{"unchanged":true,"releaseRef":"r1","digest":"d1","futureCapability":{"name":"x"}}`))
	f.Add([]byte(`{`))

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 1<<16 {
			return
		}
		var response Response
		if err := json.Unmarshal(raw, &response); err != nil {
			return
		}
		canonical, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		var roundTripped Response
		if err := json.Unmarshal(canonical, &roundTripped); err != nil {
			t.Fatalf("canonical response did not parse: %v", err)
		}
		if roundTripped.Unchanged != response.Unchanged || roundTripped.ReleaseRef != response.ReleaseRef || roundTripped.Digest != response.Digest {
			t.Fatal("known Sync response fields changed after mixed-version round trip")
		}
	})
}
