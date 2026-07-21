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

// OS-UPM-001, OS-UPM-010, OS-UPM-013, OS-UPM-054: Ubuntu Pro credentials
// remain references at the registered resource seam, while subscription
// account identity is omitted from generic reports and plans.
func TestUbuntuProFieldPolicyAndSafeProjection(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.Definition(models.ResourceKindUbuntuPro)
	if !ok {
		t.Fatal("ubuntu-pro definition is not registered")
	}
	for _, path := range []string{"tokenRef", "landscape.registrationKeyRef", "landscape.caRef"} {
		want := resourceregistry.FieldDescriptor{
			Sensitivity: resourceregistry.SensitivitySecret,
			Projection:  resourceregistry.ProjectReference,
		}
		if got := definition.FieldDescriptors[path]; got != want {
			t.Errorf("field descriptor %q = %+v, want %+v", path, got, want)
		}
	}
	if got := definition.FieldDescriptors["landscape.accountName"]; got != (resourceregistry.FieldDescriptor{
		Sensitivity: resourceregistry.SensitivitySensitiveMetadata,
		Projection:  resourceregistry.ProjectOmit,
	}) {
		t.Errorf("landscape.accountName descriptor = %+v, want sensitive metadata omitted", got)
	}

	const accountCanary = "ubuntu-pro-subscription-account-canary"
	const tokenReference = "remotr:ubuntu-pro/token@active"
	const registrationReference = "remotr:landscape/registration-key@7"
	const caReference = "remotr:landscape/ca@active"
	var document yaml.Node
	artifact := `kind: ubuntuPro
name: workstation-subscription
lifecycle: attached
tokenRef: ` + tokenReference + `
landscape:
  state: enrolled
  accountName: ` + accountCanary + `
  computerTitle: workstation
  registrationKeyRef: ` + registrationReference + `
  caRef: ` + caReference + `
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
	if strings.Contains(string(summaryWire), accountCanary) {
		t.Fatalf("safe summary leaked Landscape account identity: %s", summaryWire)
	}
	for _, reference := range []string{tokenReference, registrationReference, caReference} {
		if !strings.Contains(string(summaryWire), reference) {
			t.Errorf("safe summary omitted approved reference projection %q: %s", reference, summaryWire)
		}
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
	for _, forbidden := range []string{accountCanary, tokenReference, registrationReference, caReference} {
		if strings.Contains(string(planWire), forbidden) {
			t.Fatalf("plan descriptor leaked desired-state detail %q: %s", forbidden, planWire)
		}
	}
}
