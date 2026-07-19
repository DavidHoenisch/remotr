package resourceregistry

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"gopkg.in/yaml.v3"
)

type failingYAMLMarshaler struct{}

func (failingYAMLMarshaler) MarshalYAML() (any, error) {
	return nil, errors.New("marshal sentinel")
}

// OS-AEC-084 exercises Resource.SafeSummary as the classified projection seam.
func TestResourceSafeSummaryProjectsNestedCollectionsAndWildcardFields(t *testing.T) {
	const canary = "nested-projection-secret-canary"
	resource := Resource{
		definition: Definition{
			Kind: models.ResourceKind("test"),
			FieldDescriptors: FieldDescriptors{
				"kind":           {Sensitivity: SensitivityPublic, Projection: ProjectValue},
				"name":           {Sensitivity: SensitivityPublic, Projection: ProjectValue},
				"nested.label":   {Sensitivity: SensitivitySensitiveMetadata, Projection: ProjectMetadata},
				"items[]":        {Sensitivity: SensitivitySensitiveMetadata, Projection: ProjectCount},
				"secrets.*":      {Sensitivity: SensitivitySecret, Projection: ProjectPresence},
				"options.*.*":    {Sensitivity: SensitivitySecret, Projection: ProjectOmit},
				"unused.present": {Sensitivity: SensitivitySecret, Projection: ProjectPresence},
				"unused.count":   {Sensitivity: SensitivitySecret, Projection: ProjectCount},
			},
		},
		value: map[string]any{
			"name":   "alpha",
			"nested": map[string]any{"label": "/etc/alpha"},
			"items":  []any{"one", "two"},
			"secrets": map[string]any{
				"blank": "",
				"null":  nil,
				"set":   canary,
			},
			"options": map[string]any{
				"provider": map[string]any{"token": canary},
			},
		},
	}

	summary, err := resource.SafeSummary()
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), canary) {
		t.Fatalf("classified projection leaked nested secret: %s", wire)
	}

	fields := make(map[string]executor.SafeField, len(summary.Fields))
	for _, field := range summary.Fields {
		fields[field.Path] = field
	}
	if got := fields["kind"].Text; got != "test" {
		t.Fatalf("kind projection = %q", got)
	}
	if got := fields["name"].Text; got != "alpha" {
		t.Fatalf("name projection = %q", got)
	}
	if got := fields["nested.label"].Text; got != "/etc/alpha" {
		t.Fatalf("nested metadata projection = %q", got)
	}
	if field := fields["items[]"]; field.Count == nil || *field.Count != 2 {
		t.Fatalf("sequence count projection = %#v", field)
	}
	if field := fields["secrets.*"]; field.Present == nil || !*field.Present {
		t.Fatalf("wildcard presence projection = %#v", field)
	}
	if field := fields["unused.present"]; field.Present == nil || *field.Present {
		t.Fatalf("missing presence projection = %#v", field)
	}
	if field := fields["unused.count"]; field.Count == nil || *field.Count != 0 {
		t.Fatalf("missing count projection = %#v", field)
	}
	if _, exists := fields["options.*.*"]; exists {
		t.Fatal("omitted nested wildcard projection reached the summary")
	}
}

func TestResourceSafeSummaryRejectsUnclassifiedAcceptedField(t *testing.T) {
	resource := Resource{
		definition: Definition{
			Kind: models.ResourceKind("test"),
			FieldDescriptors: FieldDescriptors{
				"kind": {Sensitivity: SensitivityPublic, Projection: ProjectValue},
				"name": {Sensitivity: SensitivityPublic, Projection: ProjectValue},
			},
		},
		value: map[string]any{"name": "alpha", "unclassified": "secret-canary"},
	}

	summary, err := resource.SafeSummary()
	if err == nil || !strings.Contains(err.Error(), `accepted field "unclassified" has no safe projection`) {
		t.Fatalf("unclassified field error = %v", err)
	}
	if len(summary.Fields) != 0 {
		t.Fatalf("unclassified projection returned partial summary: %#v", summary)
	}
}

func TestResourceSafeSummaryPresenceDistinguishesNullBlankAndTypedValues(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{name: "null", value: nil, want: false},
		{name: "blank string", value: "", want: false},
		{name: "non-empty string", value: "set", want: true},
		{name: "integer", value: 1, want: true},
		{name: "boolean false", value: false, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := Resource{
				definition: Definition{
					Kind: models.ResourceKind("test"),
					FieldDescriptors: FieldDescriptors{
						"kind":   {Sensitivity: SensitivityPublic, Projection: ProjectValue},
						"secret": {Sensitivity: SensitivitySecret, Projection: ProjectPresence},
					},
				},
				value: map[string]any{"secret": tt.value},
			}
			summary, err := resource.SafeSummary()
			if err != nil {
				t.Fatal(err)
			}
			for _, field := range summary.Fields {
				if field.Path != "secret" {
					continue
				}
				if field.Present == nil || *field.Present != tt.want {
					t.Fatalf("presence = %#v, want %t", field.Present, tt.want)
				}
				return
			}
			t.Fatal("presence projection missing")
		})
	}
}

func TestResourceSafeSummaryReportsMarshalFailureWithoutProjecting(t *testing.T) {
	resource := Resource{
		definition: Definition{Kind: models.ResourceKind("test")},
		value:      failingYAMLMarshaler{},
	}

	_, err := resource.SafeSummary()
	if err == nil || !strings.Contains(err.Error(), "marshal sentinel") {
		t.Fatalf("marshal failure = %v", err)
	}
}

func TestResourceSafeSummaryFollowsYAMLAliasThroughClassifiedDescriptor(t *testing.T) {
	target := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "alias-value", Anchor: "safe"}
	alias := &yaml.Node{Kind: yaml.AliasNode, Value: "safe", Alias: target}
	resource := Resource{
		definition: Definition{
			Kind: models.ResourceKind("test"),
			FieldDescriptors: FieldDescriptors{
				"kind":   {Sensitivity: SensitivityPublic, Projection: ProjectValue},
				"target": {Sensitivity: SensitivitySensitiveMetadata, Projection: ProjectOmit},
				"alias":  {Sensitivity: SensitivitySensitiveMetadata, Projection: ProjectMetadata},
			},
		},
		value: &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "target"}, target,
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "alias"}, alias,
		}},
	}

	summary, err := resource.SafeSummary()
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range summary.Fields {
		if field.Path == "alias" && field.Text == "alias-value" {
			return
		}
	}
	t.Fatalf("alias projection missing from %#v", summary)
}
