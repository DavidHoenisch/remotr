package changecontrol

import (
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestApprovalPolicyRequiresDistinctAuthorizedOperators(t *testing.T) {
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	registry := NewRegistry(RegistryOptions{
		Now: func() time.Time { return now }, NewID: sequentialIDs("request", "rollout", "warning-request", "warning-rollout"),
		CanApprove: func(actor, _ string, _ models.RiskClass) bool { return actor != "unauthorized" },
	})
	request := createApprovalTestRequest(t, registry, "regulated", models.RiskDestructive)
	if request.RequiredApprovals != 2 || request.AuthorizationState != AuthorizationApprovalPending {
		t.Fatalf("request = %+v", request)
	}
	result, err := registry.ApproveRollout(request.ID, RolloutSpec{}, "operator-1", "CHG-1")
	if err != nil || result.Ready || result.ApprovalCount != 1 {
		t.Fatalf("first approval = %+v err=%v", result, err)
	}
	result, err = registry.ApproveRollout(request.ID, RolloutSpec{}, "operator-1", "CHG-1")
	if err != nil || result.Ready || result.ApprovalCount != 1 {
		t.Fatalf("duplicate identity approval = %+v err=%v", result, err)
	}
	if _, err := registry.ApproveRollout(request.ID, RolloutSpec{}, "unauthorized", "CHG-1"); err == nil {
		t.Fatal("risk-unauthorized operator approved destructive change")
	}
	result, err = registry.ApproveRollout(request.ID, RolloutSpec{}, "operator-2", "CHG-1")
	if err != nil || !result.Ready || result.Authorization.ID != "rollout" || result.ApprovalCount != 2 {
		t.Fatalf("second approval = %+v err=%v", result, err)
	}

	registry.SetApprovalPolicy(ApprovalPolicy{Fleet: map[string]map[models.RiskClass]int{
		"single-operator": {models.RiskDestructive: 1},
	}})
	warning := createApprovalTestRequest(t, registry, "single-operator", models.RiskDestructive)
	if warning.RequiredApprovals != 1 || warning.PolicyWarning != SingleOperatorDestructiveWarning {
		t.Fatalf("single-operator request = %+v", warning)
	}
	if len(registry.PolicyWarnings()) != 1 {
		t.Fatalf("policy warnings = %+v", registry.PolicyWarnings())
	}
}

func createApprovalTestRequest(t *testing.T, registry *Registry, fleet string, risk models.RiskClass) ChangeRequest {
	t.Helper()
	requests, err := registry.CreateChangeRequests(FleetPlan{
		Fleet: fleet, ReleaseRef: "release", ArtifactDigest: "artifact",
		Targets:   []TargetEvidence{{EndpointID: "endpoint", Compatible: true, PreflightReady: true}},
		Resources: []ResourcePlan{{Address: "base/resource", DesiredHash: "hash", Risk: risk, Provider: "provider"}},
	}, "creator")
	if err != nil || len(requests) != 1 {
		t.Fatalf("create request: %+v %v", requests, err)
	}
	return requests[0]
}
