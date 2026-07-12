package changecontrol

import (
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestOutcomeAccountingAndBaselinePromotionGates(t *testing.T) {
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	registry := NewRegistry(RegistryOptions{Now: func() time.Time { return now }, NewID: sequentialIDs("request", "rollout", "manual-baseline", "auto-baseline")})
	requests, err := registry.CreateChangeRequests(FleetPlan{
		Fleet: "engineering", ReleaseRef: "release", ArtifactDigest: "artifact",
		Targets:   []TargetEvidence{{EndpointID: "ok"}, {EndpointID: "blocked"}, {EndpointID: "offline"}},
		Resources: []ResourcePlan{{Address: "base/firewall", DesiredHash: "hash", Risk: models.RiskConnectivity, Provider: "nftables", BaselineEligible: true}},
	}, "creator")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.AuthorizeRollout(requests[0].ID, RolloutSpec{}, "approver", "CHG-1"); err != nil {
		t.Fatal(err)
	}
	if err := registry.RecordTargetOutcome(requests[0].ID, TargetOutcome{EndpointID: "ok", State: OutcomeVerifiedSuccess}, "agent-ok"); err != nil {
		t.Fatal(err)
	}
	if err := registry.RecordTargetOutcome(requests[0].ID, TargetOutcome{EndpointID: "blocked", State: OutcomeCapabilityBlocked, Reason: "provider unavailable"}, "agent-blocked"); err != nil {
		t.Fatal(err)
	}

	summary, err := registry.OutcomeSummary(requests[0].ID)
	if err != nil || summary.VerifiedSuccessful != 1 || summary.Blocked != 1 || summary.NotSeen != 1 || summary.FailedOrRolledBack != 0 {
		t.Fatalf("summary = %+v err=%v", summary, err)
	}
	if _, err := registry.PromoteBaselineWithOptions(requests[0].ID, "base/firewall", "operator", BaselinePromotionOptions{}); err == nil {
		t.Fatal("manual promotion ignored blocked/not-seen exceptions")
	}
	manual, err := registry.PromoteBaselineWithOptions(requests[0].ID, "base/firewall", "operator", BaselinePromotionOptions{AcknowledgeExceptions: true})
	if err != nil || manual.ID != "manual-baseline" {
		t.Fatalf("manual promotion = %+v err=%v", manual, err)
	}

	registry.SetAutomaticPromotionPolicy("engineering", AutomaticPromotionPolicy{CanaryStages: []int{1, 3}, MinimumSuccessful: 1, MaximumFailures: 0})
	if _, err := registry.TryAutomaticBaselinePromotion(requests[0].ID, "base/firewall"); err != nil {
		t.Fatalf("automatic promotion with explicit gates: %v", err)
	}
	if err := registry.RecordTargetOutcome(requests[0].ID, TargetOutcome{EndpointID: "offline", State: OutcomeFailedOrRolledBack, Reason: "rollback unresolved"}, "agent-offline"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.TryAutomaticBaselinePromotion(requests[0].ID, "base/firewall"); err == nil {
		t.Fatal("automatic promotion ignored unresolved rollback")
	}
}
