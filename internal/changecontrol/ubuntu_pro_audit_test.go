package changecontrol_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"gopkg.in/yaml.v3"
)

// OS-UPM-013: the public review and target-outcome audit lifecycle receives
// only the safe Ubuntu Pro plan projection, never enrollment identity.
func TestUbuntuProChangeControlAuditOmitsTokenIdentity(t *testing.T) {
	const tokenCanary = "ubuntu-pro-change-audit-token-canary"
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(`kind: ubuntuPro
name: primary-subscription
lifecycle: attached
tokenRef: remotr:ubuntu-pro/`+tokenCanary+`@active
services:
  - {name: esm-apps, state: enabled}
`), &document); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(document.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	plan, err := resource.PlanDescriptor("ubuntu-pro")
	if err != nil {
		t.Fatal(err)
	}
	controls := changecontrol.NewRegistry(changecontrol.RegistryOptions{NewID: func() string { return "ubuntu-pro-change" }})
	requests, err := controls.CreateChangeRequests(changecontrol.FleetPlan{
		Fleet: "production", ReleaseRef: "refs/remotr/releases/production", ArtifactDigest: "sha256:artifact",
		Targets: []changecontrol.TargetEvidence{{EndpointID: "endpoint-1", Compatible: true, PreflightReady: true}},
		Resources: []changecontrol.ResourcePlan{{
			Address: "base/primary-subscription", DesiredHash: "sha256:ubuntu-pro-safe-identity",
			Risk: resource.DefaultRisk(), Provider: "ubuntu-pro", AuthorizationGroup: resource.Metadata().AuthorizationGroup,
			RollbackClass: string(plan.RollbackClass),
		}},
	}, "operator-1")
	if err != nil || len(requests) != 1 {
		t.Fatalf("CreateChangeRequests() = %+v, %v", requests, err)
	}
	if err := controls.RecordTargetOutcome(requests[0].ID, changecontrol.TargetOutcome{
		EndpointID: "endpoint-1", State: changecontrol.OutcomeVerifiedSuccess,
	}, "agent-outcome"); err != nil {
		t.Fatal(err)
	}
	stored, ok := controls.Get(requests[0].ID)
	if !ok || len(stored.AuditHistory) != 2 || stored.AuditHistory[1].Action != changecontrol.AuditTargetOutcome {
		t.Fatalf("stored change audit = %+v", stored)
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	projection := string(encoded)
	for _, forbidden := range []string{tokenCanary, "tokenRef", "contract", "account"} {
		if strings.Contains(projection, forbidden) {
			t.Fatalf("change-control audit exposed %q: %s", forbidden, projection)
		}
	}
}
