package models

import (
	"bytes"
	"testing"
)

func FuzzParseState(f *testing.F) {
	f.Add([]byte("configurations:\n  - name: base\n"))
	f.Add([]byte("{not: yaml}"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			return
		}
		_, _ = ParseState(bytes.NewReader(data))
	})
}
