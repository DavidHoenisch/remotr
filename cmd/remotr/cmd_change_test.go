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

func TestBaselineAdoptCommandAcceptsFleetWithoutPlanFile(t *testing.T) {
	command := changeCommand()
	var adoptionFlags []string
	for _, subcommand := range command.Commands {
		if subcommand.Name != "baseline-adopt" {
			continue
		}
		for _, flag := range subcommand.Flags {
			adoptionFlags = append(adoptionFlags, flag.Names()...)
		}
	}
	if !containsString(adoptionFlags, "fleet") {
		t.Fatalf("baseline-adopt flags = %v, want fleet", adoptionFlags)
	}
	if containsString(adoptionFlags, "file") {
		t.Fatalf("baseline-adopt flags = %v, caller-authored plan file remains", adoptionFlags)
	}
}

func TestChangeRegenerateAcceptsOnlyLegacyRequestID(t *testing.T) {
	command := changeCommand()
	for _, subcommand := range command.Commands {
		if subcommand.Name != "regenerate" {
			continue
		}
		if subcommand.ArgsUsage != "<legacy-change-id>" {
			t.Fatalf("regenerate args = %q", subcommand.ArgsUsage)
		}
		var names []string
		for _, flag := range subcommand.Flags {
			names = append(names, flag.Names()...)
		}
		for _, forbidden := range []string{"file", "hash", "provider", "effect", "fleet"} {
			if containsString(names, forbidden) {
				t.Fatalf("regenerate accepts caller-authored %q: %v", forbidden, names)
			}
		}
		return
	}
	t.Fatal("change regenerate command is missing")
}

func TestChangeOutputMakesLegacyNonEnforcementVisible(t *testing.T) {
	request := admin.ChangeRequest{
		ID: "legacy-request", Fleet: "engineering", Risk: models.RiskAccess,
		AuthorizationState: changecontrol.AuthorizationActive,
		LegacyMigration: &changecontrol.LegacyAuthorizationMigration{
			Enforcement:                changecontrol.LegacyEnforcementNonEnforcing,
			Replacement:                changecontrol.LegacyReplacementRegenerated,
			Reason:                     changecontrol.LegacyReasonNoCanonicalHashContract,
			ReplacementChangeRequestID: "replacement-request",
		},
	}
	summary := captureStdout(t, func() { printChangeSummary(request) })
	if !strings.Contains(summary, "non_enforcing") || strings.Contains(summary, " authorized ") {
		t.Fatalf("legacy summary = %q", summary)
	}
	detail := captureStdout(t, func() { printChangeDetail(request) })
	for _, expected := range []string{"enforcement: non_enforcing", "replacement: regenerated", "replacement_change: replacement-request"} {
		if !strings.Contains(detail, expected) {
			t.Fatalf("legacy detail missing %q:\n%s", expected, detail)
		}
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
