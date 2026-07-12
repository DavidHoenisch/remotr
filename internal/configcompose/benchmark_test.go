package configcompose

import (
	"testing"

	"github.com/DavidHoenisch/remotr/test/benchmarkfixture"
)

var benchmarkRenderedYAML []byte

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
