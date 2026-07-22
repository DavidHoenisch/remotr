package configcompose

import (
	"fmt"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/artifactrequirements"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func FuzzTargetPredicatesAreNormalizedBoundedAndUnique(f *testing.F) {
	f.Add(uint8(1), uint8(1))
	f.Add(uint8(7), uint8(3))
	f.Fuzz(func(t *testing.T, distroBits, architectureBits uint8) {
		distros := []types.Distro{types.Ubuntu, types.Debian, types.Arch}
		architectures := []types.Architecture{types.X86, types.Arm}
		state := models.State{SchemaVersion: 1, Configurations: []models.Configuration{{Name: "portable"}}}
		for index, distro := range distros {
			if distroBits&(1<<index) != 0 {
				state.Configurations = append(state.Configurations, models.Configuration{Name: fmt.Sprintf("distro-%d", index), TargetDistros: []types.Distro{distro, distro}})
			}
		}
		for index, architecture := range architectures {
			if architectureBits&(1<<index) != 0 {
				state.Configurations = append(state.Configurations, models.Configuration{Name: fmt.Sprintf("arch-%d", index), TargetArch: []types.Architecture{architecture, architecture}})
			}
		}
		predicates, err := targetPredicates(state)
		if err != nil {
			t.Fatal(err)
		}
		if len(predicates) > MaxTargetPredicates {
			t.Fatalf("predicate count = %d", len(predicates))
		}
		seen := make(map[string]bool, len(predicates))
		for _, predicate := range predicates {
			set := artifactrequirements.Set{Version: 1, ArtifactSchemaVersion: 1, Target: predicate}
			body, err := set.CanonicalBody()
			if err != nil {
				t.Fatal(err)
			}
			key := string(body)
			if seen[key] {
				t.Fatalf("duplicate target predicate %s", key)
			}
			seen[key] = true
		}
	})
}
