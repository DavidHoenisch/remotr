package configcompose

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/test/benchmarkfixture"
)

var benchmarkRenderedYAML []byte
var benchmarkDerivedFleetPlan DerivedFleetPlan

func BenchmarkRenderFleetAndEndpoint(b *testing.B) {
	for _, size := range benchmarkfixture.Sizes() {
		root := benchmarkfixture.WriteCompositionRepository(b, size)
		for _, target := range []struct {
			name   string
			render func() ([]byte, error)
		}{
			{"fleet", func() ([]byte, error) {
				desired, _, _, _, err := RenderFleet(root, "benchmark")
				return desired, err
			}},
			{"endpoint", func() ([]byte, error) {
				desired, _, _, _, err := RenderEndpoint(root, "benchmark-endpoint")
				return desired, err
			}},
		} {
			b.Run(target.name+"/resources="+size.String(), func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					desired, err := target.render()
					if err != nil {
						b.Fatal(err)
					}
					benchmarkRenderedYAML = desired
				}
			})
		}
	}
}

func BenchmarkDerivedFleetPlanConstruction(b *testing.B) {
	ctx := context.Background()
	for _, size := range benchmarkfixture.Sizes() {
		state, err := models.ParseState(bytes.NewReader(benchmarkfixture.Schema1Artifact(size)))
		if err != nil {
			b.Fatal(err)
		}
		selections := make(map[string]ProviderSelection, int(size))
		for index := 0; index < int(size); index++ {
			selections[fmt.Sprintf("benchmark/pkg-%04d", index)] = ProviderSelection{ID: "apt"}
		}
		derived, err := DeriveFleetPlan(ctx, "benchmark", "release-1", "sha256:artifact", state, selections, nil)
		if err != nil {
			b.Fatal(err)
		}
		if len(derived.Plan.Resources) != int(size) || len(derived.TrustedIdentities) != int(size) {
			b.Fatalf("fixture plan = resources:%d identities:%d, want %d", len(derived.Plan.Resources), len(derived.TrustedIdentities), size)
		}
		b.Run("resources="+size.String(), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				plan, err := DeriveFleetPlan(ctx, "benchmark", "release-1", "sha256:artifact", state, selections, nil)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkDerivedFleetPlan = plan
			}
		})
	}
}
