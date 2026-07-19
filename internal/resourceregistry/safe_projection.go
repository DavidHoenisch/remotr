package resourceregistry

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/executor"
	"gopkg.in/yaml.v3"
)

// SafeSummary projects a desired resource through its registered field
// descriptors. Raw resource values never need to enter a generic output sink.
func (r Resource) SafeSummary() (executor.SafeSummary, error) {
	raw, err := yaml.Marshal(r.value)
	if err != nil {
		return executor.SafeSummary{}, fmt.Errorf("project %s resource: %w", r.Kind(), err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return executor.SafeSummary{}, fmt.Errorf("project %s resource: %w", r.Kind(), err)
	}
	projector := safeProjector{
		descriptors: r.definition.FieldDescriptors,
		counts:      make(map[string]int),
		presence:    make(map[string]bool),
	}
	projector.projectScalar("kind", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: string(r.Kind())})
	if len(document.Content) == 1 {
		projector.walk(document.Content[0], "")
	}
	return projector.summary()
}

type safeProjector struct {
	descriptors FieldDescriptors
	fields      []executor.SafeField
	counts      map[string]int
	presence    map[string]bool
	err         error
}

func (p *safeProjector) walk(node *yaml.Node, path string) {
	if p.err != nil || node == nil {
		return
	}
	if node.Kind == yaml.AliasNode {
		p.walk(node.Alias, path)
		return
	}
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			next := key
			if path != "" {
				next = path + "." + key
			}
			if _, exact := p.descriptors[next]; exact {
				p.walk(node.Content[i+1], next)
				continue
			}
			wildcard := wildcardPath(path)
			if wildcard != "" && p.hasDescriptorPrefix(wildcard) {
				p.walk(node.Content[i+1], wildcard)
				continue
			}
			p.walk(node.Content[i+1], next)
		}
	case yaml.SequenceNode:
		sequencePath := path + "[]"
		for _, child := range node.Content {
			p.walk(child, sequencePath)
		}
	case yaml.ScalarNode:
		p.projectScalar(path, node)
	}
}

func wildcardPath(path string) string {
	if path == "" {
		return "*"
	}
	return path + ".*"
}

func (p *safeProjector) hasDescriptorPrefix(prefix string) bool {
	for path := range p.descriptors {
		if path == prefix || strings.HasPrefix(path, prefix+".") || strings.HasPrefix(path, prefix+"[]") {
			return true
		}
	}
	return false
}

func (p *safeProjector) projectScalar(path string, node *yaml.Node) {
	descriptor, ok := p.descriptors[path]
	if !ok {
		p.err = fmt.Errorf("accepted field %q has no safe projection", path)
		return
	}
	if descriptor.Projection == ProjectOmit {
		return
	}
	sensitivity := executor.SafeSensitivity(descriptor.Sensitivity)
	projection := executor.SafeProjection(descriptor.Projection)
	switch descriptor.Projection {
	case ProjectCount:
		p.counts[path]++
	case ProjectPresence:
		p.presence[path] = p.presence[path] || (node.Tag != "!!null" && strings.TrimSpace(node.Value) != "")
	case ProjectValue, ProjectMetadata, ProjectReference, ProjectFingerprint:
		p.fields = append(p.fields, executor.SafeField{
			Path: path, Sensitivity: sensitivity, Projection: projection, Text: node.Value,
		})
	default:
		p.err = fmt.Errorf("field %q has unsupported projection %q", path, descriptor.Projection)
	}
}

func (p *safeProjector) summary() (executor.SafeSummary, error) {
	if p.err != nil {
		return executor.SafeSummary{}, p.err
	}
	paths := make([]string, 0, len(p.descriptors))
	for path := range p.descriptors {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		descriptor := p.descriptors[path]
		sensitivity := executor.SafeSensitivity(descriptor.Sensitivity)
		switch descriptor.Projection {
		case ProjectCount:
			count := p.counts[path]
			p.fields = append(p.fields, executor.SafeField{Path: path, Sensitivity: sensitivity, Projection: executor.SafeCount, Count: &count})
		case ProjectPresence:
			present := p.presence[path]
			p.fields = append(p.fields, executor.SafeField{Path: path, Sensitivity: sensitivity, Projection: executor.SafePresence, Present: &present})
		}
	}
	return executor.NewSafeSummary(p.fields)
}
