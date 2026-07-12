package registry

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/test/benchmarkfixture"
)

var benchmarkStateReportJSON []byte

func BenchmarkStateReportJSON(b *testing.B) {
	for _, size := range benchmarkfixture.Sizes() {
		report := StateReport{
			EndpointID:   "benchmark-endpoint",
			Fleet:        "benchmark",
			ReleaseRef:   "benchmark-release",
			Digest:       "benchmark-digest",
			ReportedAt:   time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
			InCompliance: false,
			Items:        benchmarkStateReportItems(int(size)),
		}
		encoded, err := json.Marshal(report)
		if err != nil {
			b.Fatal(err)
		}
		b.Run("items="+size.String(), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				payload, err := json.Marshal(report)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkStateReportJSON = payload
			}
			b.StopTimer()
			b.ReportMetric(float64(len(encoded)), "payload_bytes")
		})
	}
}

func benchmarkStateReportItems(count int) []StateReportItem {
	items := make([]StateReportItem, count)
	for i := range items {
		items[i] = StateReportItem{
			Address:     fmt.Sprintf("benchmark/resource-%04d", i),
			Name:        fmt.Sprintf("resource-%04d", i),
			Description: "deterministic benchmark drift finding",
		}
	}
	return items
}
