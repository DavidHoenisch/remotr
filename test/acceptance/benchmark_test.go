package acceptance

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/test/benchmarkfixture"
)

var benchmarkRedacted string

func BenchmarkRedactFailureAttachment(b *testing.B) {
	for _, size := range benchmarkfixture.Sizes() {
		payload := strings.Repeat("token=remotr-test-secret-canary api_key=remotr-test-secret-canary ", int(size))
		b.Run("resources="+size.String(), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(payload)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkRedacted = redact(payload)
			}
		})
	}
}
