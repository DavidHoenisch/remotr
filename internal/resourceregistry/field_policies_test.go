package resourceregistry

import (
	"slices"
	"sort"
	"testing"
)

// This is the schema-completeness linter for OS-AEC-083. Its expected paths
// come from the exact typed schema used by KnownFields decoding, never from the
// policy table being checked.
func TestDefaultFieldPoliciesCoverStrictSchemas(t *testing.T) {
	registry, err := NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	for kind, definition := range registry.definitions {
		t.Run(string(kind), func(t *testing.T) {
			if _, ok := explicitFieldPolicies[kind]; !ok {
				t.Fatalf("kind %q has no explicit field policy", kind)
			}
			want := acceptedFieldPaths(definition.schemaType)
			got := make([]string, 0, len(definition.FieldDescriptors))
			for path, descriptor := range definition.FieldDescriptors {
				if !descriptor.valid() {
					t.Errorf("field %q has invalid descriptor %+v", path, descriptor)
				}
				got = append(got, path)
			}
			sort.Strings(got)
			if !slices.Equal(got, want) {
				t.Fatalf("descriptor paths = %v, strict schema paths = %v", got, want)
			}
		})
	}
}
