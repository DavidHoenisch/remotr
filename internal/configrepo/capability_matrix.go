package configrepo

import (
	"fmt"

	"github.com/DavidHoenisch/remotr/internal/capabilitymatrix"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
)

func validateCapabilityMatrix(state models.State) error {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		return err
	}
	for i := range state.Configurations {
		configuration := &state.Configurations[i]
		resources, err := registry.Resources(configuration)
		if err != nil {
			return err
		}
		for _, resource := range resources {
			if err := capabilitymatrix.ValidateStatic(state.SchemaVersion, *configuration, resource.Value()); err != nil {
				return fmt.Errorf("resource %q: %w", models.ResourceAddress(configuration.Name, resource.Name()), err)
			}
		}
	}
	return nil
}
