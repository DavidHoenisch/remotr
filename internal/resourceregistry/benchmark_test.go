package resourceregistry_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/test/benchmarkfixture"
)

var benchmarkClassifiedResources []byte

func BenchmarkClassifiedResourceSerialization(b *testing.B) {
	for _, size := range benchmarkfixture.Sizes() {
		state, err := models.ParseState(bytes.NewReader(benchmarkfixture.Schema1Artifact(size)))
		if err != nil {
			b.Fatal(err)
		}
		registry, err := resourceregistry.NewDefault()
		if err != nil {
			b.Fatal(err)
		}
		resources, err := registry.Resources(&state.Configurations[0])
		if err != nil {
			b.Fatal(err)
		}
		if len(resources) != int(size) {
			b.Fatalf("fixture resources = %d, want %d", len(resources), size)
		}
		b.Run("resources="+size.String(), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				summaries := make([]executor.SafeSummary, len(resources))
				for index, resource := range resources {
					summary, err := resource.SafeSummary()
					if err != nil {
						b.Fatal(err)
					}
					summaries[index] = summary
				}
				encoded, err := json.Marshal(summaries)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkClassifiedResources = encoded
			}
		})
	}
}
