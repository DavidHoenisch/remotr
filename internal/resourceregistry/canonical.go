package resourceregistry

import (
	"bytes"
	"fmt"
	"time"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
	"gopkg.in/yaml.v3"
)

type canonicalArtifact struct {
	SchemaVersion  int                      `yaml:"schemaVersion"`
	Kind           types.Kind               `yaml:"kind,omitempty"`
	Configurations []canonicalConfiguration `yaml:"configurations"`
}

type canonicalConfiguration struct {
	Name          string               `yaml:"name"`
	Description   string               `yaml:"description,omitempty"`
	LastUpdated   time.Time            `yaml:"lastUpdated,omitempty"`
	TargetDistros []types.Distro       `yaml:"targetDistros,omitempty"`
	TargetArch    []types.Architecture `yaml:"targetArch,omitempty"`
	Resources     []*yaml.Node         `yaml:"resources,omitempty"`
}

// MarshalCanonical emits the deterministic schema-1 resource-list form.
func MarshalCanonical(state models.State) ([]byte, error) {
	registry, err := NewDefault()
	if err != nil {
		return nil, err
	}
	artifact := canonicalArtifact{SchemaVersion: 1, Kind: state.Kind}
	for i := range state.Configurations {
		input := &state.Configurations[i]
		output := canonicalConfiguration{
			Name: input.Name, Description: input.Description, LastUpdated: input.LastUpdated,
			TargetDistros: input.TargetDistros, TargetArch: input.TargetArch,
		}
		resources, err := registry.Resources(input)
		if err != nil {
			return nil, err
		}
		for _, resource := range resources {
			node, err := resource.canonicalNode()
			if err != nil {
				return nil, fmt.Errorf("resource %q: %w", models.ResourceAddress(input.Name, resource.Name()), err)
			}
			output.Resources = append(output.Resources, node)
		}
		artifact.Configurations = append(artifact.Configurations, output)
	}
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(artifact); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func (r Resource) canonicalNode() (*yaml.Node, error) {
	raw, err := yaml.Marshal(r.value)
	if err != nil {
		return nil, err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("encoded resource is not a mapping")
	}
	mapping := document.Content[0]
	if r.Kind() == models.ResourceKindPackage {
		mapping, err = removeMappingKey(mapping, "present")
		if err != nil {
			return nil, err
		}
	}
	kindKey := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "kind"}
	kindValue := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: string(r.Kind())}
	mapping.Content = append([]*yaml.Node{kindKey, kindValue}, mapping.Content...)
	return mapping, nil
}
