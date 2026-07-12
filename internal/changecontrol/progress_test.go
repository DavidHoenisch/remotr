package changecontrol

import (
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestRiskSpecificAcknowledgementEvidence(t *testing.T) {
	tests := []struct {
		name           string
		risk           models.RiskClass
		invalid, valid RiskEvidence
	}{
		{"network", models.RiskConnectivity, RiskEvidence{AuthenticatedSync: true}, RiskEvidence{WatchdogArmed: true, AuthenticatedSync: true}},
		{"access", models.RiskAccess, RiskEvidence{TechnicalValidation: true, RequireHumanCanary: true}, RiskEvidence{TechnicalValidation: true, RequireHumanCanary: true, HumanCanaryVerified: true}},
		{"reboot", models.RiskBoot, RiskEvidence{PriorBootID: "boot-1", CurrentBootID: "boot-1"}, RiskEvidence{PriorBootID: "boot-1", CurrentBootID: "boot-2"}},
		{"destructive", models.RiskDestructive, RiskEvidence{StableDeviceIdentity: "disk-1"}, RiskEvidence{StableDeviceIdentity: "disk-1", PostconditionsVerified: true, RollbackClass: "none"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			registry, lease := leaseForRisk(t, tc.risk)
			if _, err := registry.UpdateExecutionProgress(lease.ID, ProgressUpdate{State: ProgressAcknowledged, Evidence: tc.invalid}); err == nil {
				t.Fatal("invalid acknowledgement evidence was accepted")
			}
			updated, err := registry.UpdateExecutionProgress(lease.ID, ProgressUpdate{State: ProgressAcknowledged, Evidence: tc.valid})
			if err != nil || updated.Progress != ProgressAcknowledged {
				t.Fatalf("updated = %+v err=%v", updated, err)
			}
		})
	}
}

func leaseForRisk(t *testing.T, risk models.RiskClass) (*Registry, ExecutionLease) {
	t.Helper()
	registry := NewRegistry(RegistryOptions{NewID: sequentialIDs("request", "rollout", "lease")})
	requests, err := registry.CreateChangeRequests(FleetPlan{
		Fleet: "fleet", ReleaseRef: "release", ArtifactDigest: "artifact",
		Targets:   []TargetEvidence{{EndpointID: "endpoint", Compatible: true, PreflightReady: true}},
		Resources: []ResourcePlan{{Address: "base/resource", DesiredHash: "hash", Risk: risk, Provider: "provider"}},
	}, "creator")
	if err != nil {
		t.Fatal(err)
	}
	if risk == models.RiskDestructive {
		if _, err := registry.ApproveRollout(requests[0].ID, RolloutSpec{}, "one", "CHG"); err != nil {
			t.Fatal(err)
		}
		if _, err := registry.ApproveRollout(requests[0].ID, RolloutSpec{}, "two", "CHG"); err != nil {
			t.Fatal(err)
		}
	} else if _, err := registry.AuthorizeRollout(requests[0].ID, RolloutSpec{}, "one", "CHG"); err != nil {
		t.Fatal(err)
	}
	lease, issued, err := registry.IssueExecutionLease(requests[0].ID, PreflightReport{EndpointID: "endpoint", Ready: true})
	if err != nil || !issued {
		t.Fatalf("lease=%+v issued=%t err=%v", lease, issued, err)
	}
	return registry, lease
}
