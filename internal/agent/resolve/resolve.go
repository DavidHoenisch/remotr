package resolve

import (
	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/types"
	"gopkg.in/yaml.v3"
)

// ResolvedState is desired state after in-document targeting.
type ResolvedState struct {
	Configurations  []models.Configuration
	ResourceSources map[string]yaml.Node
}

var defaultRegistry = func() *resourceregistry.Registry {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		panic(err)
	}
	return registry
}()

// Resolve filters configurations and nested resources for local facts.
func Resolve(state models.State, f facts.Facts) ResolvedState {
	resolved, err := ResolveWithRegistry(state, f, defaultRegistry)
	if err != nil {
		panic(err)
	}
	return resolved
}

// ResolveWithRegistry filters configurations and resources through registered contracts.
func ResolveWithRegistry(state models.State, f facts.Facts, registry *resourceregistry.Registry) (ResolvedState, error) {
	out := ResolvedState{ResourceSources: make(map[string]yaml.Node)}
	for _, cfg := range state.Configurations {
		if !matchesDistro(cfg.TargetDistros, f.Distro) {
			continue
		}
		if !matchesArch(cfg.TargetArch, f.Arch) {
			continue
		}
		resolved := models.Configuration{
			Name:        cfg.Name,
			Description: cfg.Description,
			LastUpdated: cfg.LastUpdated,
		}
		pm := facts.PackageManagerForDistro(f.Distro)
		resources, err := registry.Resources(&cfg)
		if err != nil {
			return ResolvedState{}, err
		}
		for _, resource := range resources {
			if pkg, ok := resource.Value().(*models.Package); ok {
				if pkg.PM != "" && types.IsDistroSpecificPackageManager(pkg.PM) && pkg.PM != pm {
					continue
				}
				if pkg.Arch != "" && pkg.Arch != f.Arch {
					continue
				}
			}
			if err := resource.AppendTo(&resolved); err != nil {
				return ResolvedState{}, err
			}
			address := models.ResourceAddress(cfg.Name, resource.Name())
			if source, ok := state.ResourceSources[address]; ok {
				out.ResourceSources[address] = source
			}
		}
		out.Configurations = append(out.Configurations, resolved)
	}
	return out, nil
}

func matchesDistro(targets []types.Distro, d types.Distro) bool {
	if len(targets) == 0 {
		return true
	}
	for _, t := range targets {
		if t == d {
			return true
		}
	}
	return false
}

func matchesArch(targets []types.Architecture, a types.Architecture) bool {
	if len(targets) == 0 {
		return true
	}
	for _, t := range targets {
		if t == a {
			return true
		}
	}
	return false
}
