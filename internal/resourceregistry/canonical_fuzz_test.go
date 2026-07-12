package resourceregistry_test

import (
	"bytes"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
)

func FuzzCanonicalArtifactRoundTrip(f *testing.F) {
	f.Add([]byte("schemaVersion: 1\nconfigurations:\n  - name: base\n    resources:\n      - kind: package\n        name: curl\n        present: true\n"))
	f.Add([]byte("configurations:\n  - name: base\n    files:\n      - name: motd\n        path: /etc/motd\n        content: managed\n"))
	f.Add([]byte("schemaVersion: 2\nconfigurations: []\n"))

	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 1<<20 {
			return
		}
		state, err := models.ParseState(bytes.NewReader(input))
		if err != nil {
			return
		}
		canonical, err := resourceregistry.MarshalCanonical(state)
		if err != nil {
			t.Fatal(err)
		}
		roundTripped, err := models.ParseState(bytes.NewReader(canonical))
		if err != nil {
			t.Fatalf("canonical artifact did not parse: %v\n%s", err, canonical)
		}
		recanonical, err := resourceregistry.MarshalCanonical(roundTripped)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(canonical, recanonical) {
			t.Fatalf("canonical artifact changed after round trip\n--- first ---\n%s--- second ---\n%s", canonical, recanonical)
		}
	})
}
