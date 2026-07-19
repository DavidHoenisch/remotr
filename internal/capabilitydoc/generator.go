package capabilitydoc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/providerregistry"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
)

// Generator derives capability evidence from the same immutable registries
// used to parse resources and choose runtime providers.
type Generator struct {
	resources              *resourceregistry.Registry
	providers              *providerregistry.Registry
	artifactSchemaVersions []int
}

// NewDefaultGenerator constructs the modern agent generator from all currently
// registered resource and provider contracts.
func NewDefaultGenerator(artifactSchemaVersions []int) (*Generator, error) {
	resources, err := resourceregistry.NewDefault()
	if err != nil {
		return nil, fmt.Errorf("resource capability registry: %w", err)
	}
	providers, err := providerregistry.NewDefault()
	if err != nil {
		return nil, fmt.Errorf("provider capability registry: %w", err)
	}
	return &Generator{
		resources:              resources,
		providers:              providers,
		artifactSchemaVersions: append([]int(nil), artifactSchemaVersions...),
	}, nil
}

// Generate captures current normalized endpoint facts and the contracts that
// those facts make available. The resulting document includes its canonical
// digest and is ready for local validation.
func (g *Generator) Generate(endpoint facts.Facts, agentVersion string) (Document, error) {
	if g == nil || g.resources == nil || g.providers == nil {
		return Document{}, fmt.Errorf("capability document generator is not configured")
	}
	endpoint = endpoint.Normalized()
	document := Document{
		DocumentVersion:        CurrentDocumentVersion,
		ArtifactSchemaVersions: append([]int(nil), g.artifactSchemaVersions...),
		AgentVersion:           agentVersion,
	}
	for _, definition := range g.resources.Definitions() {
		document.Capabilities = append(document.Capabilities, Capability{
			ID:       "resource:" + string(definition.Kind),
			Revision: definition.ProviderContractRevision,
		})
	}
	for _, definition := range g.providers.Definitions(endpoint) {
		document.Capabilities = append(document.Capabilities, Capability{
			ID:       "provider:" + string(definition.Capability) + "/" + definition.ID,
			Revision: definition.ContractRevision,
		})
	}
	document.Facts = normalizedFacts(endpoint)
	sort.Slice(document.Capabilities, func(i, j int) bool {
		if document.Capabilities[i].ID == document.Capabilities[j].ID {
			return document.Capabilities[i].Revision < document.Capabilities[j].Revision
		}
		return document.Capabilities[i].ID < document.Capabilities[j].ID
	})
	sort.Slice(document.Facts, func(i, j int) bool {
		if document.Facts[i].Key == document.Facts[j].Key {
			return document.Facts[i].Value < document.Facts[j].Value
		}
		return document.Facts[i].Key < document.Facts[j].Key
	})
	return document.WithCanonicalDigest()
}

func normalizedFacts(endpoint facts.Facts) []Fact {
	values := []Fact{
		{Key: "distro", Value: lower(endpoint.Distro)},
		{Key: "distro-family", Value: lower(endpoint.DistroFamily)},
		{Key: "distro-version", Value: lower(endpoint.DistroVersion)},
		{Key: "architecture", Value: lower(endpoint.Arch)},
		{Key: "init", Value: lower(endpoint.Init)},
		{Key: "package", Value: lower(endpoint.Package)},
		{Key: "firewall", Value: lower(endpoint.Firewall)},
		{Key: "network", Value: lower(endpoint.Network)},
		{Key: "security", Value: lower(endpoint.Security)},
	}
	for _, backend := range endpoint.Desktop {
		values = append(values, Fact{Key: "desktop", Value: lower(backend)})
	}
	for _, browser := range endpoint.Browser {
		values = append(values, Fact{Key: "browser", Value: lower(browser)})
	}
	out := values[:0]
	for _, fact := range values {
		if fact.Value != "" {
			out = append(out, fact)
		}
	}
	return out
}

func lower(value any) string {
	return strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
}
