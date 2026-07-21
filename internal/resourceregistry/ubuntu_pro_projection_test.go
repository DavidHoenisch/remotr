package resourceregistry_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"gopkg.in/yaml.v3"
)

// OS-UPM-001, OS-UPM-010, and OS-UPM-013: Ubuntu Pro credentials remain
// references at the registered resource seam.
func TestUbuntuProFieldPolicyAndSafeProjection(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.Definition(models.ResourceKindUbuntuPro)
	if !ok {
		t.Fatal("ubuntu-pro definition is not registered")
	}
	want := resourceregistry.FieldDescriptor{
		Sensitivity: resourceregistry.SensitivitySecret,
		Projection:  resourceregistry.ProjectReference,
	}
	if got := definition.FieldDescriptors["tokenRef"]; got != want {
		t.Errorf("tokenRef descriptor = %+v, want %+v", got, want)
	}

	const tokenReference = "remotr:ubuntu-pro/token@active"
	var document yaml.Node
	artifact := `kind: ubuntuPro
name: workstation-subscription
lifecycle: attached
tokenRef: ` + tokenReference + `
`
	if err := yaml.Unmarshal([]byte(artifact), &document); err != nil {
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
	summaryWire, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(summaryWire), tokenReference) {
		t.Errorf("safe summary omitted approved reference projection %q: %s", tokenReference, summaryWire)
	}
	for _, field := range summary.Fields {
		if field.Path == "tokenRef" || strings.HasSuffix(field.Path, "Ref") {
			if field.Sensitivity != executor.SafeSecret || field.Projection != executor.SafeReference {
				t.Errorf("reference field escaped reference-only projection: %+v", field)
			}
		}
	}

	plan, err := resource.PlanDescriptor("ubuntu-pro")
	if err != nil {
		t.Fatal(err)
	}
	planWire, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(planWire), tokenReference) {
		t.Fatalf("plan descriptor leaked desired-state detail %q: %s", tokenReference, planWire)
	}
}
