package changecontrol

import (
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestExecutionLeaseSchedulingHonorsConcurrencyExpiryPauseAndRevoke(t *testing.T) {
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	registry := NewRegistry(RegistryOptions{Now: func() time.Time { return now }, NewID: sequentialIDs("request", "rollout", "lease-a", "lease-b")})
	requests, err := registry.CreateChangeRequests(FleetPlan{
		Fleet: "engineering", ReleaseRef: "release", ArtifactDigest: "artifact",
		Targets:   []TargetEvidence{{EndpointID: "endpoint-a", Compatible: true, PreflightReady: true}, {EndpointID: "endpoint-b", Compatible: true, PreflightReady: true}},
		Resources: []ResourcePlan{{Address: "base/firewall", DesiredHash: "hash", Risk: models.RiskConnectivity, Provider: "nftables"}},
	}, "creator")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.AuthorizeRollout(requests[0].ID, RolloutSpec{MaxConcurrency: 1, AttemptLimit: 2}, "approver", "CHG-1"); err != nil {
		t.Fatal(err)
	}

	leaseA, issued, err := registry.IssueExecutionLease(requests[0].ID, PreflightReport{EndpointID: "endpoint-a", Ready: true})
	if err != nil || !issued || leaseA.ID != "lease-a" || leaseA.Attempt != 1 || leaseA.ResourceHashes["base/firewall"] != "hash" || !leaseA.ExpiresAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("lease A = %+v issued=%t err=%v", leaseA, issued, err)
	}
	if _, issued, err := registry.IssueExecutionLease(requests[0].ID, PreflightReport{EndpointID: "endpoint-b", Ready: true}); err != nil || issued {
		t.Fatalf("concurrency lease issued=%t err=%v", issued, err)
	}
	if _, err := registry.Pause(requests[0].ID, "operator"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(6 * time.Minute)
	if _, issued, err := registry.IssueExecutionLease(requests[0].ID, PreflightReport{EndpointID: "endpoint-b", Ready: true}); err != nil || issued {
		t.Fatalf("paused lease issued=%t err=%v", issued, err)
	}
	if _, err := registry.Resume(requests[0].ID, "operator"); err != nil {
		t.Fatal(err)
	}
	leaseB, issued, err := registry.IssueExecutionLease(requests[0].ID, PreflightReport{EndpointID: "endpoint-b", Ready: true})
	if err != nil || !issued || leaseB.ID != "lease-b" {
		t.Fatalf("lease B = %+v issued=%t err=%v", leaseB, issued, err)
	}
	if _, err := registry.Revoke(requests[0].ID, "operator"); err != nil {
		t.Fatal(err)
	}
	if _, issued, err := registry.IssueExecutionLease(requests[0].ID, PreflightReport{EndpointID: "endpoint-a", Ready: true}); err != nil || issued {
		t.Fatalf("revoked lease issued=%t err=%v", issued, err)
	}
}
