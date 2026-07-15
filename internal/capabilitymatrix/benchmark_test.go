package capabilitymatrix

import (
	"fmt"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
	"github.com/DavidHoenisch/remotr/test/benchmarkfixture"
)

var benchmarkCapabilityRequirements []string
var benchmarkCapabilityErr error

func BenchmarkCapabilitySelection(b *testing.B) {
	configuration := models.Configuration{Name: "benchmark", TargetDistros: []types.Distro{types.Debian, types.Ubuntu}}
	endpoint := facts.Facts{Distro: types.Debian, Package: types.Apt}
	for _, size := range benchmarkfixture.Sizes() {
		resources := make([]models.Package, size)
		for i := range resources {
			resources[i] = models.Package{Name: fmt.Sprintf("package-%04d", i), Present: true, PM: types.Apt}
		}
		b.Run("resources="+size.String(), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for index := range resources {
					resource := &resources[index]
					if err := ValidateStatic(1, configuration, resource); err != nil {
						b.Fatal(err)
					}
					benchmarkCapabilityRequirements = Requirements(models.ResourceKindPackage, resource)
					benchmarkCapabilityErr = CheckRuntime(resource, endpoint)
					if benchmarkCapabilityErr != nil {
						b.Fatal(benchmarkCapabilityErr)
					}
				}
			}
		})
	}
}
