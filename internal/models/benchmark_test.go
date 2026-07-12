package models

import (
	"bytes"
	"testing"

	"github.com/DavidHoenisch/remotr/test/benchmarkfixture"
)

var benchmarkState State

func BenchmarkParseState(b *testing.B) {
	for _, size := range benchmarkfixture.Sizes() {
		raw := benchmarkfixture.Artifact(size)
		b.Run("resources="+size.String(), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(raw)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				state, err := ParseState(bytes.NewReader(raw))
				if err != nil {
					b.Fatal(err)
				}
				benchmarkState = state
			}
		})
	}
}
