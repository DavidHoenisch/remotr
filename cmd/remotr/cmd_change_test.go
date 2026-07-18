package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestChangeOutputSupportsHumanAndJSONContracts(t *testing.T) {
	const canary = "change-cli-secret-canary"
	present := true
	request := admin.ChangeRequest{
		ID: "change-1", Fleet: "engineering", ReleaseRef: "release-1",
		AuthorizationGroup: "network-transition", Risk: models.RiskConnectivity,
		AuthorizationState: changecontrol.AuthorizationPending,
		Resources: []changecontrol.ResourcePlan{{
			Address: "base/firewall", DesiredHash: "hash-1", Risk: models.RiskConnectivity,
			PredictedEffects: []changecontrol.PredictedEffect{{
				Code: changecontrol.EffectResourceUpdate,
				Details: executor.SafeSummary{Fields: []executor.SafeField{{
					Path: "content", Sensitivity: executor.SafeSecret, Projection: executor.SafePresence, Present: &present,
				}}},
			}},
		}},
		FrozenTargets: []changecontrol.TargetEvidence{{EndpointID: "endpoint-a", Compatible: true, PreflightReady: true}},
	}
	human := captureStdout(t, func() { printChangeDetail(request) })
	for _, want := range []string{"change: change-1", "group: network-transition", "base/firewall  hash-1", "resource_update: content=true", "endpoint-a  compatible=true"} {
		if !strings.Contains(human, want) {
			t.Fatalf("human output missing %q:\n%s", want, human)
		}
	}
	if strings.Contains(human, canary) {
		t.Fatalf("human output leaked canary:\n%s", human)
	}
	raw := captureStdout(t, func() {
		if err := encodeJSON(request); err != nil {
			t.Fatal(err)
		}
	})
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["id"] != "change-1" || decoded["authorization_state"] != "pending" {
		t.Fatalf("JSON output = %s", raw)
	}
	if strings.Contains(raw, canary) {
		t.Fatalf("JSON output leaked canary: %s", raw)
	}
}
