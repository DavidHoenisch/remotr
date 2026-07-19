package resourceregistry

import (
	"reflect"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

type fuzzClassificationEntry struct {
	Key   string `yaml:"key"`
	Token string `yaml:"token"`
}

type fuzzClassificationSchema struct {
	Name    string                       `yaml:"name"`
	Secret  string                       `yaml:"secret"`
	Entries []fuzzClassificationEntry    `yaml:"entries"`
	Options map[string]map[string]string `yaml:"options"`
	Ignored string                       `yaml:"-"`
}

type ignoredNestedClassificationSchema struct {
	Name    string                           `yaml:"name"`
	Options ignoredNestedClassificationValue `yaml:"options"`
}

type ignoredNestedClassificationValue struct {
	Ignored string `yaml:"-"`
}

func TestRegistryTreatsNonSerializingNestedStructAsOneAcceptedField(t *testing.T) {
	registry, err := NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.Definition(models.ResourceKindFile)
	if !ok {
		t.Fatal("file definition is not registered")
	}
	definition.schemaType = reflect.TypeOf(ignoredNestedClassificationSchema{})
	definition.FieldDescriptors = FieldDescriptors{
		"kind":    {Sensitivity: SensitivityPublic, Projection: ProjectValue},
		"name":    {Sensitivity: SensitivityPublic, Projection: ProjectValue},
		"options": {Sensitivity: SensitivitySensitiveMetadata, Projection: ProjectOmit},
	}
	if _, err := New(definition); err != nil {
		t.Fatalf("New() rejected the complete public registration policy: %v", err)
	}
}

// FuzzSchemaClassificationRejectsIncompleteOrInvalidPolicies checks the
// registration classifier against a fixed independently enumerated strict
// schema. A policy is accepted exactly when every accepted leaf is present,
// every descriptor pair is valid, and no unknown path is claimed.
func FuzzSchemaClassificationRejectsIncompleteOrInvalidPolicies(f *testing.F) {
	f.Add(uint8(0x3f), uint8(0), uint8(0), "")
	f.Add(uint8(0x3e), uint8(0), uint8(0), "")
	f.Add(uint8(0x3f), uint8(2), uint8(2), "")
	f.Add(uint8(0x3f), uint8(1), uint8(1), "unknown")

	f.Fuzz(func(t *testing.T, mask, rawSensitivity, rawProjection uint8, extraPath string) {
		if len(extraPath) > 128 {
			return
		}
		paths := []string{"kind", "name", "secret", "entries[].key", "entries[].token", "options.*.*"}
		sensitivities := []Sensitivity{
			SensitivityPublic, SensitivitySensitiveMetadata, SensitivitySecret, "", "invalid",
		}
		projections := []SafeProjection{
			ProjectValue, ProjectMetadata, ProjectReference, ProjectFingerprint,
			ProjectPresence, ProjectCount, ProjectOmit, "", "invalid",
		}
		descriptor := FieldDescriptor{
			Sensitivity: sensitivities[int(rawSensitivity)%len(sensitivities)],
			Projection:  projections[int(rawProjection)%len(projections)],
		}
		fields := make(FieldDescriptors, len(paths)+1)
		for index, path := range paths {
			if mask&(1<<uint(index)) != 0 {
				fields[path] = descriptor
			}
		}
		if extraPath != "" {
			fields[extraPath] = descriptor
		}

		known := make(map[string]struct{}, len(paths))
		wantAccepted := classificationPairIsValid(descriptor)
		for _, path := range paths {
			known[path] = struct{}{}
			if _, ok := fields[path]; !ok {
				wantAccepted = false
			}
		}
		for path := range fields {
			if _, ok := known[path]; !ok {
				wantAccepted = false
			}
		}

		err := validateFieldDescriptors(
			models.ResourceKindFile,
			reflect.TypeOf(fuzzClassificationSchema{}),
			fields,
		)
		if (err == nil) != wantAccepted {
			t.Fatalf("classification acceptance=%t, want %t: mask=%06b descriptor=%+v extra=%q err=%v", err == nil, wantAccepted, mask&0x3f, descriptor, extraPath, err)
		}
	})
}

func classificationPairIsValid(descriptor FieldDescriptor) bool {
	switch descriptor.Sensitivity {
	case SensitivityPublic:
		return descriptor.Projection == ProjectValue
	case SensitivitySensitiveMetadata:
		switch descriptor.Projection {
		case ProjectMetadata, ProjectFingerprint, ProjectPresence, ProjectCount, ProjectOmit:
			return true
		}
	case SensitivitySecret:
		switch descriptor.Projection {
		case ProjectReference, ProjectPresence, ProjectCount, ProjectOmit:
			return true
		}
	}
	return false
}
