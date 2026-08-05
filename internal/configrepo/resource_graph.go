package configrepo

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
)

type graphResource struct {
	kind            models.ResourceKind
	lifecycle       models.Lifecycle
	dependsOn       []string
	trustReferences []string
}

func validateResourceGraph(state models.State, requireDependencyTargets bool) error {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		return err
	}
	resources := make(map[string]graphResource)
	for i := range state.Configurations {
		configuration := &state.Configurations[i]
		registered, err := registry.Resources(configuration)
		if err != nil {
			return err
		}
		for _, resource := range registered {
			address := models.ResourceAddress(configuration.Name, resource.Name())
			if previous, exists := resources[address]; exists {
				// Schema-0 package variants may share a name when their distro
				// providers are mutually exclusive. Canonical schema 1 has one
				// stable resource identity and rejects every duplicate.
				if state.SchemaVersion == 0 && previous.kind == resource.Kind() {
					continue
				}
				return fmt.Errorf("configuration %q: duplicate resource address %q across kinds %q and %q", configuration.Name, address, previous.kind, resource.Kind())
			}
			resources[address] = graphResource{
				kind: resource.Kind(), lifecycle: resource.Metadata().Lifecycle,
				dependsOn:       append([]string(nil), resource.Metadata().DependsOn...),
				trustReferences: policyTrustReferences(resource.Value()),
			}
		}
	}

	addresses := make([]string, 0, len(resources))
	for address := range resources {
		addresses = append(addresses, address)
	}
	sort.Strings(addresses)
	for _, address := range addresses {
		for _, dependency := range resources[address].dependsOn {
			if err := validateStableAddress(dependency); err != nil {
				return fmt.Errorf("resource %q dependency %q: %w", address, dependency, err)
			}
			if _, exists := resources[dependency]; !exists && requireDependencyTargets {
				return fmt.Errorf("resource %q has unknown dependency %q", address, dependency)
			}
		}
		for _, reference := range resources[address].trustReferences {
			target, exists := resources[reference]
			if !exists {
				continue
			}
			if target.kind != models.ResourceKindTrustAnchor {
				return fmt.Errorf("resource %q trust reference %q targets %q, not trustAnchor", address, reference, target.kind)
			}
			if target.lifecycle == models.LifecycleAbsent {
				return fmt.Errorf("resource %q trust reference %q targets an absent trustAnchor", address, reference)
			}
		}
	}
	return validateDependencyCycles(resources, addresses)
}

func policyTrustReferences(value any) []string {
	switch resource := value.(type) {
	case *models.BrowserPolicyResource:
		return append([]string(nil), resource.TrustAnchors...)
	case *models.SessionPolicyResource:
		return append([]string(nil), resource.TrustAnchors...)
	default:
		return nil
	}
}

func validateStableAddress(address string) error {
	configuration, resource, ok := strings.Cut(address, "/")
	if !ok || strings.TrimSpace(configuration) == "" || strings.TrimSpace(resource) == "" ||
		configuration != strings.TrimSpace(configuration) || resource != strings.TrimSpace(resource) {
		return fmt.Errorf("must use stable <configuration>/<resource-name> address")
	}
	return nil
}

func validateDependencyCycles(resources map[string]graphResource, addresses []string) error {
	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[string]int, len(resources))
	stack := make([]string, 0, len(resources))
	var visit func(string) error
	visit = func(address string) error {
		switch state[address] {
		case visiting:
			start := 0
			for i := range stack {
				if stack[i] == address {
					start = i
					break
				}
			}
			cycle := append(append([]string(nil), stack[start:]...), address)
			return fmt.Errorf("dependency cycle detected: %s", strings.Join(cycle, " -> "))
		case visited:
			return nil
		}
		state[address] = visiting
		stack = append(stack, address)
		dependencies := append([]string(nil), resources[address].dependsOn...)
		sort.Strings(dependencies)
		for _, dependency := range dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		state[address] = visited
		return nil
	}
	for _, address := range addresses {
		if err := visit(address); err != nil {
			return err
		}
	}
	return nil
}
