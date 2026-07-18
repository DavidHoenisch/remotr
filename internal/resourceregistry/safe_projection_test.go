package resourceregistry_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"gopkg.in/yaml.v3"
)

// OS-AEC-074: the registered field policy, rather than provider-authored
// prose, decides what desired state can enter a generic sink.
func TestResourceSafeSummaryProjectsClassifiedFieldsAndOmitsSecretCanary(t *testing.T) {
	const canary = "resource-safe-projection-secret-canary"
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte("kind: file\nname: managed\npath: /etc/managed.conf\nmode: [416]\ncontent: "+canary+"\n"), &document); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(document.Content[0])
	if err != nil {
		t.Fatal(err)
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
		t.Fatalf("classified summary leaked secret: %s", wire)
	}

	want := map[string]executor.SafeField{
		"kind": {Path: "kind", Sensitivity: executor.SafePublic, Projection: executor.SafeValue, Text: "file"},
		"path": {Path: "path", Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata, Text: "/etc/managed.conf"},
	}
	for _, field := range summary.Fields {
		if expected, ok := want[field.Path]; ok && field == expected {
			delete(want, field.Path)
		}
		if field.Path == "content" {
			t.Fatalf("omitted content field reached summary: %+v", field)
		}
	}
	if len(want) != 0 {
		t.Fatalf("classified fields missing from summary: %+v; summary=%+v", want, summary)
	}
}
