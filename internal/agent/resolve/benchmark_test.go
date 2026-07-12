package resolve_test

import (
	"fmt"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/resolve"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
	"github.com/DavidHoenisch/remotr/test/benchmarkfixture"
)

var benchmarkResolvedState resolve.ResolvedState

func BenchmarkResolveRegisteredResources(b *testing.B) {
	for _, size := range benchmarkfixture.Sizes() {
		packages := make([]models.Package, 0, size)
		for i := 0; i < int(size); i++ {
			packages = append(packages, models.Package{Name: fmt.Sprintf("package-%04d", i), Present: true, PM: types.Apt})
		}
		state := models.State{Configurations: []models.Configuration{{Name: "benchmark", Packages: packages}}}
		b.Run("resources="+size.String(), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkResolvedState = resolve.Resolve(state, facts.Facts{Distro: types.Debian, Arch: types.X86})
			}
		})
	}
}
