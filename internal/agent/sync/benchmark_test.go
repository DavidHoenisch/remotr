package sync

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"testing"

	"github.com/DavidHoenisch/remotr/test/benchmarkfixture"
)

var benchmarkSyncPayload []byte
var benchmarkUnchanged bool

func BenchmarkSyncResponseJSONAndGzip(b *testing.B) {
	for _, size := range benchmarkfixture.Sizes() {
		response := Response{
			ReleaseRef:        "benchmark-release",
			Digest:            "benchmark-digest",
			ArtifactYAML:      benchmarkfixture.Artifact(size),
			RemediationPolicy: "report",
		}
		raw, err := json.Marshal(response)
		if err != nil {
			b.Fatal(err)
		}

		b.Run("json/resources="+size.String(), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(raw)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				payload, err := json.Marshal(response)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkSyncPayload = payload
			}
			b.StopTimer()
			b.ReportMetric(float64(len(raw)), "payload_bytes")
		})

		compressedPayload := benchmarkGzip(b, raw)
		b.Run("gzip/resources="+size.String(), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(raw)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var compressed bytes.Buffer
				writer := gzip.NewWriter(&compressed)
				if _, err := writer.Write(raw); err != nil {
					b.Fatal(err)
				}
				if err := writer.Close(); err != nil {
					b.Fatal(err)
				}
				benchmarkSyncPayload = compressed.Bytes()
			}
			b.StopTimer()
			b.ReportMetric(float64(len(compressedPayload)), "payload_bytes")
		})
	}
}

func benchmarkGzip(b testing.TB, raw []byte) []byte {
	b.Helper()
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(raw); err != nil {
		b.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		b.Fatal(err)
	}
	return compressed.Bytes()
}

func BenchmarkUnchangedSuppression(b *testing.B) {
	for _, tc := range []struct {
		name                                         string
		lastDigest, serverDigest, lastRef, serverRef string
	}{
		{"matching-digest-and-release", "digest", "digest", "release", "release"},
		{"matching-digest-empty-release", "digest", "digest", "", ""},
		{"release-advanced", "digest", "digest", "old", "new"},
		{"digest-changed", "old", "new", "release", "release"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				benchmarkUnchanged = Unchanged(tc.lastDigest, tc.serverDigest, tc.lastRef, tc.serverRef)
			}
		})
	}
}
