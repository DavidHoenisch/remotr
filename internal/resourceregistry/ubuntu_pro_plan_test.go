package resourceregistry_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/providercontract"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"gopkg.in/yaml.v3"
)

// OS-UPM-050 through OS-UPM-053: Ubuntu Pro plan risk is computed from the
// highest-impact declared operation and cannot be lowered by authored risk.
func TestUbuntuProDynamicRiskAndSafePlan(t *testing.T) {
	const tokenCanary = "ubuntu-pro-plan-token-canary"
	tests := []struct {
		name         string
		yaml         string
		wantRisk     models.RiskClass
		wantRollback providercontract.RollbackClass
	}{
		{name: "attachment", yaml: "lifecycle: attached\ntokenRef: remotr:ubuntu-pro/" + tokenCanary + "@active\n", wantRisk: models.RiskSensitive, wantRollback: providercontract.RollbackBestEffort},
		{name: "ordinary enable", yaml: "lifecycle: attached\ntokenRef: remotr:ubuntu-pro/token@active\nservices:\n  - {name: esm-apps, state: enabled}\n", wantRisk: models.RiskSensitive, wantRollback: providercontract.RollbackBestEffort},
		{name: "fips boot", yaml: "lifecycle: attached\nrisk: normal\ntokenRef: remotr:ubuntu-pro/token@active\nservices:\n  - {name: fips, state: enabled}\n", wantRisk: models.RiskBoot, wantRollback: providercontract.RollbackBestEffort},
		{name: "kernel boot", yaml: "lifecycle: attached\ntokenRef: remotr:ubuntu-pro/token@active\nservices:\n  - {name: realtime-kernel, state: enabled, variant: raspi}\n", wantRisk: models.RiskBoot, wantRollback: providercontract.RollbackBestEffort},
		{name: "disable destructive", yaml: "lifecycle: attached\ntokenRef: remotr:ubuntu-pro/token@active\nservices:\n  - {name: esm-apps, state: disabled}\n", wantRisk: models.RiskDestructive, wantRollback: providercontract.RollbackBestEffort},
		{name: "purge destructive", yaml: "lifecycle: attached\ntokenRef: remotr:ubuntu-pro/token@active\nservices:\n  - {name: esm-apps, state: disabled, disableMode: purge}\n", wantRisk: models.RiskDestructive, wantRollback: providercontract.RollbackNone},
		{name: "detach destructive", yaml: "lifecycle: detached\n", wantRisk: models.RiskDestructive, wantRollback: providercontract.RollbackNone},
		{name: "authored escalation", yaml: "lifecycle: attached\nrisk: destructive\ntokenRef: remotr:ubuntu-pro/token@active\n", wantRisk: models.RiskDestructive, wantRollback: providercontract.RollbackBestEffort},
	}
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var document yaml.Node
			if err := yaml.Unmarshal([]byte("kind: ubuntuPro\nname: primary-subscription\n"+test.yaml), &document); err != nil {
				t.Fatal(err)
			}
			resource, err := registry.Decode(document.Content[0])
			if err != nil {
				t.Fatal(err)
			}
			if err := resource.Validate(); err != nil {
				t.Fatal(err)
			}
			if risk := resource.DefaultRisk(); risk != test.wantRisk {
				t.Fatalf("DefaultRisk() = %q, want %q", risk, test.wantRisk)
			}
			plan, err := resource.PlanDescriptor("ubuntu-pro")
			if err != nil {
				t.Fatal(err)
			}
			if plan.RollbackClass != test.wantRollback {
				t.Fatalf("PlanDescriptor().RollbackClass = %q, want %q", plan.RollbackClass, test.wantRollback)
			}
			projectionBytes, err := json.Marshal(plan)
			if err != nil {
				t.Fatal(err)
			}
			projection := string(projectionBytes)
			if strings.Contains(projection, tokenCanary) || strings.Contains(projection, "tokenRef") || strings.Contains(projection, "contract") {
				t.Fatalf("plan exposed protected subscription detail: %s", projection)
			}
		})
	}
}
