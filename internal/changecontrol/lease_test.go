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

// OS-AEC-045 and OS-AEC-047: completion releases the concurrency slot while
// recurring windows independently decide whether the next lease is eligible.
func TestExecutionLeaseCompletionReleasesConcurrencyOnlyInsideWindow(t *testing.T) {
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC) // Saturday.
	registry := NewRegistry(RegistryOptions{Now: func() time.Time { return now }, NewID: sequentialIDs("request", "rollout", "lease-a", "lease-b")})
	requests, err := registry.CreateChangeRequests(FleetPlan{
		Fleet: "engineering", ReleaseRef: "release", ArtifactDigest: "artifact",
		Targets:   []TargetEvidence{{EndpointID: "endpoint-a", Compatible: true, PreflightReady: true}, {EndpointID: "endpoint-b", Compatible: true, PreflightReady: true}},
		Resources: []ResourcePlan{{Address: "base/firewall", DesiredHash: "hash", Risk: models.RiskConnectivity, Provider: "nftables"}},
	}, "creator")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.AuthorizeRollout(requests[0].ID, RolloutSpec{
		AttemptLimit:   2,
		MaxConcurrency: 1,
		ExecutionWindows: []RecurringWindow{{
			Weekdays: []time.Weekday{time.Saturday}, StartMinuteUTC: 19 * 60, Duration: 2 * time.Hour,
		}},
	}, "approver", "CHG-2"); err != nil {
		t.Fatal(err)
	}

	leaseA, issued, err := registry.IssueExecutionLease(requests[0].ID, PreflightReport{EndpointID: "endpoint-a", Ready: true})
	if err != nil || !issued {
		t.Fatalf("first lease issued=%t err=%v", issued, err)
	}
	if err := registry.CompleteExecutionLease(leaseA.ID); err != nil {
		t.Fatal(err)
	}
	leaseB, issued, err := registry.IssueExecutionLease(requests[0].ID, PreflightReport{EndpointID: "endpoint-b", Ready: true})
	if err != nil || !issued || leaseB.ID != "lease-b" {
		t.Fatalf("lease after completion = %+v issued=%t err=%v", leaseB, issued, err)
	}

	now = now.Add(24 * time.Hour) // Sunday, outside the recurring window.
	if _, issued, err := registry.IssueExecutionLease(requests[0].ID, PreflightReport{EndpointID: "endpoint-a", Ready: true}); err != nil || issued {
		t.Fatalf("outside-window lease issued=%t err=%v", issued, err)
	}
}

func TestCanonicalExecutionLeaseRejectsPreflightHashMismatch(t *testing.T) {
	now := time.Date(2035, 3, 4, 5, 6, 7, 0, time.UTC)
	const currentHash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const staleHash = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	registry := NewRegistry(RegistryOptions{Now: func() time.Time { return now }, NewID: sequentialIDs("request", "rollout", "lease")})
	plan := FleetPlan{
		Fleet: "engineering", ReleaseRef: "release", ArtifactDigest: "sha256:artifact",
		HashContractVersion: 1,
		Targets:             []TargetEvidence{{EndpointID: "endpoint", Compatible: true, PreflightReady: true}},
		Resources: []ResourcePlan{{
			Address: "base/firewall", DesiredHash: currentHash, Risk: models.RiskConnectivity,
			Provider: "nftables", ProviderRevision: "firewall-v1",
		}},
	}
	requests, err := registry.CreateCanonicalChangeRequests(plan, []CanonicalResourceIdentity{{
		Address: "base/firewall", EffectiveHash: currentHash, Provider: "nftables",
		ProviderRevision: "firewall-v1", HashContractVersion: 1,
	}}, "creator")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.AuthorizeRollout(requests[0].ID, RolloutSpec{}, "approver", "CHG-HASH"); err != nil {
		t.Fatal(err)
	}
	if _, issued, err := registry.IssueExecutionLease(requests[0].ID, PreflightReport{
		EndpointID: "endpoint", Ready: true, ResourceHashes: map[string]string{"base/firewall": staleHash},
	}); err == nil || issued {
		t.Fatalf("mismatched preflight lease issued=%t err=%v", issued, err)
	}
	lease, issued, err := registry.IssueExecutionLease(requests[0].ID, PreflightReport{
		EndpointID: "endpoint", Ready: true, ResourceHashes: map[string]string{"base/firewall": currentHash},
	})
	if err != nil || !issued || lease.HashContractVersion != 1 || lease.ResourceHashes["base/firewall"] != currentHash {
		t.Fatalf("canonical lease = %+v issued=%t err=%v", lease, issued, err)
	}
}

// OS-AEC-087: a current preflight cannot override safety evidence that was
// already blocked when the canonical target set was frozen.
func TestExecutionLeaseRejectsFrozenRollbackReservationBlock(t *testing.T) {
	now := time.Date(2035, 3, 4, 5, 6, 7, 0, time.UTC)
	const currentHash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	registry := NewRegistry(RegistryOptions{Now: func() time.Time { return now }, NewID: sequentialIDs("request", "rollout", "must-not-lease")})
	plan := FleetPlan{
		Fleet: "engineering", ReleaseRef: "release", ArtifactDigest: "sha256:artifact",
		HashContractVersion: 1,
		Targets: []TargetEvidence{{
			EndpointID: "endpoint", Compatible: true, PreflightReady: false,
			PreflightReason: "rollback_reservation_failed",
		}},
		Resources: []ResourcePlan{{
			Address: "base/firewall", DesiredHash: currentHash, Risk: models.RiskConnectivity,
			Provider: "firewall", ProviderRevision: "firewall-v1", RollbackClass: "transactional",
		}},
	}
	requests, err := registry.CreateCanonicalChangeRequests(plan, []CanonicalResourceIdentity{{
		Address: "base/firewall", EffectiveHash: currentHash, Provider: "firewall",
		ProviderRevision: "firewall-v1", HashContractVersion: 1,
	}}, "creator")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.AuthorizeRollout(requests[0].ID, RolloutSpec{}, "approver", "CHG-SAFETY"); err != nil {
		t.Fatal(err)
	}
	if _, issued, err := registry.IssueExecutionLease(requests[0].ID, PreflightReport{
		EndpointID: "endpoint", Ready: true, ResourceHashes: map[string]string{"base/firewall": currentHash},
	}); err != nil || issued {
		t.Fatalf("blocked frozen target lease issued=%t err=%v", issued, err)
	}
}
