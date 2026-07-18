package resourceregistry

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"github.com/DavidHoenisch/remotr/internal/models"
)

// FieldDescriptor is the exposure contract for one leaf accepted by strict
// resource decoding. Safe projection policy is added separately; a missing or
// invalid sensitivity is never inferred at registration time.
type FieldDescriptor struct {
	Sensitivity Sensitivity
	Projection  SafeProjection
}

// FieldDescriptors maps canonical YAML paths to their exposure contracts.
// Sequences use [] and arbitrary map keys use * (for example entries[].key and
// providerOptions.*.*).
type FieldDescriptors map[string]FieldDescriptor

// SafeProjection defines the only value shape a field may contribute outside
// the raw execution boundary.
type SafeProjection string

const (
	ProjectValue       SafeProjection = "value"
	ProjectMetadata    SafeProjection = "metadata"
	ProjectReference   SafeProjection = "reference"
	ProjectFingerprint SafeProjection = "fingerprint"
	ProjectPresence    SafeProjection = "presence"
	ProjectCount       SafeProjection = "count"
	ProjectOmit        SafeProjection = "omit"
)

func (p SafeProjection) valid() bool {
	switch p {
	case ProjectValue, ProjectMetadata, ProjectReference, ProjectFingerprint, ProjectPresence, ProjectCount, ProjectOmit:
		return true
	default:
		return false
	}
}

func (d FieldDescriptor) valid() bool {
	if !d.Sensitivity.Valid() || !d.Projection.valid() {
		return false
	}
	switch d.Sensitivity {
	case SensitivityPublic:
		return d.Projection == ProjectValue
	case SensitivitySensitiveMetadata:
		return d.Projection == ProjectMetadata || d.Projection == ProjectFingerprint ||
			d.Projection == ProjectPresence || d.Projection == ProjectCount || d.Projection == ProjectOmit
	case SensitivitySecret:
		return d.Projection == ProjectReference || d.Projection == ProjectPresence ||
			d.Projection == ProjectCount || d.Projection == ProjectOmit
	default:
		return false
	}
}

func cloneFieldDescriptors(fields FieldDescriptors) FieldDescriptors {
	cloned := make(FieldDescriptors, len(fields))
	for path, descriptor := range fields {
		cloned[path] = descriptor
	}
	return cloned
}

func validateFieldDescriptors(kind models.ResourceKind, schema reflect.Type, fields FieldDescriptors) error {
	accepted := acceptedFieldPaths(schema)
	if len(accepted) == 0 {
		return fmt.Errorf("resource kind %q has no accepted schema fields", kind)
	}
	acceptedSet := make(map[string]struct{}, len(accepted))
	for _, path := range accepted {
		acceptedSet[path] = struct{}{}
		descriptor, ok := fields[path]
		if !ok || !descriptor.valid() {
			return fmt.Errorf("resource kind %q field %q has no valid sensitivity classification", kind, path)
		}
	}
	for path, descriptor := range fields {
		if _, ok := acceptedSet[path]; !ok {
			return fmt.Errorf("resource kind %q has descriptor for unknown field %q", kind, path)
		}
		if !descriptor.valid() {
			return fmt.Errorf("resource kind %q field %q has invalid sensitivity/projection %q/%q", kind, path, descriptor.Sensitivity, descriptor.Projection)
		}
	}
	return nil
}

func acceptedFieldPaths(schema reflect.Type) []string {
	paths := map[string]struct{}{"kind": {}}
	walkAcceptedFields(indirectType(schema), "", paths, make(map[reflect.Type]bool))
	out := make([]string, 0, len(paths))
	for path := range paths {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func walkAcceptedFields(schema reflect.Type, prefix string, paths map[string]struct{}, stack map[reflect.Type]bool) {
	if schema == nil || schema.Kind() != reflect.Struct || stack[schema] {
		return
	}
	stack[schema] = true
	defer delete(stack, schema)
	for i := 0; i < schema.NumField(); i++ {
		field := schema.Field(i)
		if field.PkgPath != "" {
			continue
		}
		name, options := yamlField(field)
		if name == "-" {
			continue
		}
		if options["inline"] {
			walkAcceptedFields(indirectType(field.Type), prefix, paths, stack)
			continue
		}
		path := joinFieldPath(prefix, name)
		walkAcceptedType(field.Type, path, paths, stack)
	}
}

func walkAcceptedType(fieldType reflect.Type, path string, paths map[string]struct{}, stack map[reflect.Type]bool) {
	fieldType = indirectType(fieldType)
	if fieldType == nil {
		return
	}
	switch fieldType.Kind() {
	case reflect.Slice, reflect.Array:
		element := indirectType(fieldType.Elem())
		path += "[]"
		if isYAMLStruct(element) {
			walkAcceptedFields(element, path, paths, stack)
		} else {
			paths[path] = struct{}{}
		}
	case reflect.Map:
		element := indirectType(fieldType.Elem())
		path += ".*"
		if isYAMLStruct(element) {
			walkAcceptedFields(element, path, paths, stack)
		} else if element != nil && element.Kind() == reflect.Map {
			walkAcceptedType(element, path, paths, stack)
		} else {
			paths[path] = struct{}{}
		}
	case reflect.Struct:
		if isYAMLStruct(fieldType) {
			walkAcceptedFields(fieldType, path, paths, stack)
		} else {
			paths[path] = struct{}{}
		}
	default:
		paths[path] = struct{}{}
	}
}

func indirectType(value reflect.Type) reflect.Type {
	for value != nil && value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	return value
}

func isYAMLStruct(value reflect.Type) bool {
	value = indirectType(value)
	if value == nil || value.Kind() != reflect.Struct {
		return false
	}
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		if field.PkgPath == "" && field.Tag.Get("yaml") != "-" {
			return true
		}
	}
	return false
}

func yamlField(field reflect.StructField) (string, map[string]bool) {
	parts := strings.Split(field.Tag.Get("yaml"), ",")
	name := parts[0]
	if name == "" {
		name = lowerFieldName(field.Name)
	}
	options := make(map[string]bool, len(parts)-1)
	for _, option := range parts[1:] {
		options[option] = true
	}
	return name, options
}

func lowerFieldName(name string) string {
	if name == "" {
		return name
	}
	runes := []rune(name)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func joinFieldPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}
