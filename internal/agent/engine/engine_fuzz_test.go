package engine_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/resolve"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func FuzzEngineDependencyOrdering(f *testing.F) {
	f.Add(uint8(0))
	f.Add(uint8(0b111))
	f.Add(uint8(0b011))

	f.Fuzz(func(t *testing.T, edges uint8) {
		dependsOn := func(bit uint8, address string) []string {
			if edges&(1<<bit) != 0 {
				return []string{address}
			}
			return nil
		}
		state := resolve.ResolvedState{Configurations: []models.Configuration{{
			Name: "cfg",
			Packages: []models.Package{
				{Name: "a", Present: true, ResourceMeta: models.ResourceMeta{DependsOn: dependsOn(0, "cfg/b")}},
				{Name: "b", Present: true, ResourceMeta: models.ResourceMeta{DependsOn: dependsOn(1, "cfg/c")}},
				{Name: "c", Present: true, ResourceMeta: models.ResourceMeta{DependsOn: dependsOn(2, "cfg/a")}},
			},
		}}}
		eng, err := engine.New(state, facts.Facts{Distro: types.Debian, Arch: types.X86}, nil, nil)
		if err != nil {
			if !strings.Contains(err.Error(), "cycle") {
				t.Fatalf("dependency resolution error = %v", err)
			}
			return
		}
		order := eng.NodeOrder()
		index := make(map[string]int, len(order))
		for i, address := range order {
			index[address] = i
		}
		if edges&1 != 0 && index["cfg/b"] >= index["cfg/a"] || edges&2 != 0 && index["cfg/c"] >= index["cfg/b"] || edges&4 != 0 && index["cfg/a"] >= index["cfg/c"] {
			t.Fatalf("dependency order %v violates declared edges %03b", order, edges)
		}
	})
}
