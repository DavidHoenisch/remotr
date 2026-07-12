package models

import (
	"bytes"
	"testing"

	"gopkg.in/yaml.v3"
)

func FuzzParseState(f *testing.F) {
	f.Add([]byte("configurations:\n  - name: base\n"))
	f.Add([]byte("{not: yaml}"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			return
		}
		state, err := ParseState(bytes.NewReader(data))
		if err != nil {
			return
		}
		canonical, err := yaml.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		roundTripped, err := ParseState(bytes.NewReader(canonical))
		if err != nil {
			t.Fatalf("canonical state did not parse: %v", err)
		}
		recanonical, err := yaml.Marshal(roundTripped)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(recanonical, canonical) {
			t.Fatal("canonical state changed after parse round trip")
		}
	})
}
