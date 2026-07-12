// Package resourceregistry centralizes desired-state resource contracts.
package resourceregistry

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/apppackages"
	"github.com/DavidHoenisch/remotr/internal/capabilitymatrix"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"gopkg.in/yaml.v3"
)

// Sensitivity classifies how resource fields may be exposed in diagnostics.
type Sensitivity string

const (
	SensitivityPublic            Sensitivity = "public"
	SensitivitySensitiveMetadata Sensitivity = "sensitive-metadata"
	SensitivitySecret            Sensitivity = "secret"
)

// Valid reports whether the sensitivity classification is declared.
func (s Sensitivity) Valid() bool {
	return s == SensitivityPublic || s == SensitivitySensitiveMetadata || s == SensitivitySecret
}

// FactoryContext supplies endpoint and external-boundary dependencies to providers.
type FactoryContext struct {
	Facts       facts.Facts
	Runner      executil.Runner
	PackageURLs apppackages.URLResolver
	SyncURL     string
}

// Definition is the complete contract registered for one resource kind.
type Definition struct {
	Kind            models.ResourceKind
	Decode          func(*yaml.Node) (any, error)
	Validate        func(any) error
	Metadata        func(any) (string, *models.ResourceMeta, error)
	Sensitivity     Sensitivity
	DefaultRisk     func(any) models.RiskClass
	ProviderFactory func(any, FactoryContext) (executor.Handler, error)
	OrderingTier    func(any) int
	LockDomains     func(any) []string
	List            func(*models.Configuration) []any
	Append          func(*models.Configuration, any) error
}

func (d Definition) complete() error {
	if !d.Kind.Valid() {
		return fmt.Errorf("invalid resource kind %q", d.Kind)
	}
	if d.Decode == nil || d.Validate == nil || d.Metadata == nil || d.DefaultRisk == nil ||
		d.ProviderFactory == nil || d.OrderingTier == nil || d.LockDomains == nil ||
		d.List == nil || d.Append == nil || !d.Sensitivity.Valid() {
		return fmt.Errorf("resource kind %q has an incomplete definition", d.Kind)
	}
	return nil
}

// Registry stores one immutable definition per kind.
type Registry struct {
	definitions map[models.ResourceKind]Definition
	order       []models.ResourceKind
}

// New validates and constructs a registry.
func New(definitions ...Definition) (*Registry, error) {
	registry := &Registry{definitions: make(map[models.ResourceKind]Definition, len(definitions))}
	for _, definition := range definitions {
		if err := definition.complete(); err != nil {
			return nil, err
		}
		if _, exists := registry.definitions[definition.Kind]; exists {
			return nil, fmt.Errorf("duplicate resource kind %q", definition.Kind)
		}
		registry.definitions[definition.Kind] = definition
		registry.order = append(registry.order, definition.Kind)
	}
	sort.SliceStable(registry.order, func(i, j int) bool {
		a, b := registry.definitions[registry.order[i]], registry.definitions[registry.order[j]]
		return a.OrderingTier(nil) < b.OrderingTier(nil)
	})
	return registry, nil
}

// Definitions returns a stable copy of all registered contracts.
func (r *Registry) Definitions() []Definition {
	out := make([]Definition, 0, len(r.order))
	for _, kind := range r.order {
		out = append(out, r.definitions[kind])
	}
	return out
}

// Definition returns the contract for kind.
func (r *Registry) Definition(kind models.ResourceKind) (Definition, bool) {
	definition, ok := r.definitions[kind]
	return definition, ok
}

// Decode strictly decodes one canonical resource through its registered contract.
func (r *Registry) Decode(node *yaml.Node) (Resource, error) {
	if node == nil {
		return Resource{}, fmt.Errorf("resource node is required")
	}
	var header models.ResourceHeader
	if err := node.Decode(&header); err != nil {
		return Resource{}, err
	}
	definition, ok := r.definitions[header.Kind]
	if !ok {
		return Resource{}, fmt.Errorf("unknown resource kind %q", header.Kind)
	}
	value, err := definition.Decode(node)
	if err != nil {
		return Resource{}, err
	}
	name, metadata, err := definition.Metadata(value)
	if err != nil {
		return Resource{}, err
	}
	metadata.Kind = header.Kind
	if header.Name != name {
		return Resource{}, fmt.Errorf("resource header name %q does not match decoded name %q", header.Name, name)
	}
	return Resource{definition: definition, value: value, name: name, metadata: metadata}, nil
}

// Resources returns every registered resource in a configuration.
func (r *Registry) Resources(configuration *models.Configuration) ([]Resource, error) {
	var resources []Resource
	for _, definition := range r.Definitions() {
		for _, value := range definition.List(configuration) {
			name, metadata, err := definition.Metadata(value)
			if err != nil {
				return nil, err
			}
			metadata.Kind = definition.Kind
			resources = append(resources, Resource{definition: definition, value: value, name: name, metadata: metadata})
		}
	}
	return resources, nil
}

// Resource binds one typed desired-state value to its registered contract.
type Resource struct {
	definition Definition
	value      any
	name       string
	metadata   *models.ResourceMeta
}

func (r Resource) Kind() models.ResourceKind      { return r.definition.Kind }
func (r Resource) Name() string                   { return r.name }
func (r Resource) Value() any                     { return r.value }
func (r Resource) Metadata() *models.ResourceMeta { return r.metadata }
func (r Resource) Sensitivity() Sensitivity       { return r.definition.Sensitivity }
func (r Resource) DefaultRisk() models.RiskClass  { return r.definition.DefaultRisk(r.value) }
func (r Resource) OrderingTier() int              { return r.definition.OrderingTier(r.value) }
func (r Resource) LockDomains() []string          { return r.definition.LockDomains(r.value) }
func (r Resource) Validate() error                { return r.definition.Validate(r.value) }
func (r Resource) AppendTo(configuration *models.Configuration) error {
	return r.definition.Append(configuration, r.value)
}
func (r Resource) NewProvider(ctx FactoryContext) (executor.Handler, error) {
	if ctx.Runner == nil {
		ctx.Runner = executil.OSRunner{}
	}
	if err := capabilitymatrix.CheckRuntime(r.value, ctx.Facts); err != nil {
		return unsupportedProvider{name: r.name, reason: err}, nil
	}
	return r.definition.ProviderFactory(r.value, ctx)
}

func strictDecodeResource[T any](node *yaml.Node) (any, error) {
	withoutKind, err := removeMappingKey(node, "kind")
	if err != nil {
		return nil, err
	}
	raw, err := yaml.Marshal(withoutKind)
	if err != nil {
		return nil, err
	}
	var value T
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return &value, nil
}

func removeMappingKey(node *yaml.Node, key string) (*yaml.Node, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("resource must be a mapping")
	}
	clone := *node
	clone.Content = make([]*yaml.Node, 0, len(node.Content))
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			continue
		}
		clone.Content = append(clone.Content, node.Content[i], node.Content[i+1])
	}
	return &clone, nil
}

func validateMetadata(name string, metadata *models.ResourceMeta) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("resource name is required")
	}
	return metadata.ValidateCanonical()
}
