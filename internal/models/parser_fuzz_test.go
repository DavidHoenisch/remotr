package models_test

import (
	"bytes"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
)

func FuzzParseState(f *testing.F) {
	f.Add([]byte("configurations:\n  - name: base\n"))
	f.Add([]byte("schemaVersion: 2\nconfigurations: []\n"))
	f.Add([]byte("schemaVersion: 1\nconfigurations:\n  - name: base\n    resources:\n      - kind: package\n        name: curl\n        presnt: true\n"))
	f.Add([]byte("schemaVersion: 1\nconfigurations:\n  - name: base\n    resources:\n      - kind: kernelModule\n        name: loop\n        module: loop\n        loaded: false\n        persistent: true\n"))
	f.Add([]byte("schemaVersion: 1\nconfigurations:\n  - name: base\n    resources:\n      - kind: hostLocale\n        name: berlin\n        timezone: Europe/Berlin\n        locale:\n          LANG: de_DE.UTF-8\n"))
	f.Add([]byte("schemaVersion: 1\nconfigurations:\n  - name: base\n    resources:\n      - kind: timeSync\n        name: ntp\n        provider: systemd-timesyncd\n        enabled: true\n        servers: [time.example.test]\n"))
	f.Add([]byte("schemaVersion: 1\nconfigurations:\n  - name: base\n    resources:\n      - kind: mount\n        name: cache\n        source: tmpfs\n        target: /var/cache/remotr\n        filesystemType: tmpfs\n        options: [mode=0755]\n        mounted: true\n        persistent: true\n"))
	f.Add([]byte("{not: yaml}"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			return
		}
		state, err := models.ParseState(bytes.NewReader(data))
		if err != nil {
			return
		}
		canonical, err := resourceregistry.MarshalCanonical(state)
		if err != nil {
			t.Fatal(err)
		}
		roundTripped, err := models.ParseState(bytes.NewReader(canonical))
		if err != nil {
			t.Fatalf("canonical state did not parse: %v", err)
		}
		recanonical, err := resourceregistry.MarshalCanonical(roundTripped)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(recanonical, canonical) {
			t.Fatal("canonical state changed after parse round trip")
		}
	})
}
