package apppackages

import (
	"bytes"
	"testing"

	"gopkg.in/yaml.v3"
)

func FuzzParseManifestSchemaVersion(f *testing.F) {
	f.Add([]byte("schemaVersion: 1\nname: demo\nversion: v1\ninstall:\n  mode: script\n  script: [install]\n"))
	f.Add([]byte("schemaVersion: 0\nname: demo\nversion: v1\ninstall:\n  mode: script\n  script: [install]\n"))
	f.Add([]byte("schemaVersion: 2\n"))

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 1<<16 {
			return
		}
		manifest, err := ParseManifest(raw)
		if err != nil {
			return
		}
		canonical, err := yaml.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		roundTripped, err := ParseManifest(canonical)
		if err != nil {
			t.Fatalf("canonical manifest did not parse: %v", err)
		}
		recanonical, err := yaml.Marshal(roundTripped)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(canonical, recanonical) {
			t.Fatal("manifest canonical form changed after parse round trip")
		}
	})
}
