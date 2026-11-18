package configrepo

import (
	"fmt"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
)

// PreflightDiagnostics warns for every resource whose effective risk requires a
// provider preflight that its provider does not implement. The agent engine
// refuses to apply such a resource, so it would stay blocked forever.
func PreflightDiagnostics(state models.State) []models.Diagnostic {
	registered, err := resourceregistry.NewDefault()
	if err != nil {
		return nil
	}
	var diagnostics []models.Diagnostic
	for index := range state.Configurations {
		configuration := &state.Configurations[index]
		resources, err := registered.Resources(configuration)
		if err != nil {
			continue
		}
		for _, resource := range resources {
			risk := resource.Metadata().EffectiveRisk(resource.DefaultRisk())
			if !risk.RequiresPreflight() {
				continue
			}
			capable, err := resource.ProvidesPreflight()
			if err != nil || capable {
				continue
			}
			diagnostics = append(diagnostics, models.Diagnostic{
				Code: models.DiagnosticUnsatisfiablePreflight,
				Message: fmt.Sprintf("%s has effective risk %s, which requires a preflight that the %s provider does not implement, so the agent can never apply it; declare risk: normal if the change is routine",
					models.ResourceAddress(configuration.Name, resource.Name()), risk, resource.Kind()),
			})
		}
	}
	return diagnostics
}
