package changecontrol

import (
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestRegistryRolloutAndBaselineAuthorizationAreHashBound(t *testing.T) {
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC) // Saturday.
	registry := NewRegistry(RegistryOptions{Now: func() time.Time { return now }, NewID: sequentialIDs("request", "rollout", "baseline")})
	requests, err := registry.CreateChangeRequests(FleetPlan{
		Fleet: "engineering", ReleaseRef: "release-1", ArtifactDigest: "artifact-1",
		Targets: []TargetEvidence{{EndpointID: "endpoint-a", Compatible: true, PreflightReady: true}},
		Resources: []ResourcePlan{
			{Address: "base/firewall", DesiredHash: "hash-firewall", Risk: models.RiskConnectivity, Provider: "nftables", AuthorizationGroup: "guarded", RollbackClass: "transactional", BaselineEligible: true},
			{Address: "base/reboot", DesiredHash: "hash-reboot", Risk: models.RiskBoot, Provider: "systemd", AuthorizationGroup: "guarded", RollbackClass: "none"},
		},
	}, "operator-1")
	if err != nil || len(requests) != 1 {
		t.Fatalf("create request: requests=%+v err=%v", requests, err)
	}

	rollout, err := registry.AuthorizeRollout(requests[0].ID, RolloutSpec{
		AttemptLimit:   2,
		MaxConcurrency: 1,
		ExecutionWindows: []RecurringWindow{{
			Weekdays: []time.Weekday{time.Saturday}, StartMinuteUTC: 19 * 60, Duration: 2 * time.Hour,
		}},
	}, "operator-1", "CHG-42")
	if err != nil {
		t.Fatal(err)
	}
	if !rollout.ValidUntil.Equal(now.Add(30*24*time.Hour)) || rollout.ResourceHashes["base/firewall"] != "hash-firewall" || len(rollout.FrozenTargets) != 1 {
		t.Fatalf("rollout = %+v", rollout)
	}
	if !registry.RolloutActive(requests[0].ID, now) {
		t.Fatal("rollout should be active inside recurring window")
	}
	if registry.RolloutActive(requests[0].ID, now.Add(24*time.Hour)) {
		t.Fatal("rollout should be inactive outside recurring window")
	}

	baseline, err := registry.PromoteBaseline(requests[0].ID, "base/firewall", "operator-2")
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Fleet != "engineering" || baseline.DesiredHash != "hash-firewall" || baseline.Provider != "nftables" || !baseline.Active() {
		t.Fatalf("baseline = %+v", baseline)
	}
	if _, err := registry.PromoteBaseline(requests[0].ID, "base/reboot", "operator-2"); err == nil {
		t.Fatal("reboot event became a baseline")
	}
	if !registry.BaselineAuthorizes("engineering", "base/firewall", "hash-firewall", "nftables", true) {
		t.Fatal("matching new endpoint should use baseline after current preflight")
	}
	if registry.BaselineAuthorizes("engineering", "base/firewall", "hash-firewall", "nftables", false) || registry.BaselineAuthorizes("engineering", "base/firewall", "changed", "nftables", true) || registry.BaselineAuthorizes("engineering", "base/firewall", "hash-firewall", "firewalld", true) {
		t.Fatal("baseline authorized failed preflight, changed hash, or changed provider")
	}

	invalidated, err := registry.InvalidateBaselines("engineering", "base/firewall", "changed", "operator-3")
	if err != nil {
		t.Fatal(err)
	}
	if len(invalidated) != 1 || invalidated[0].Active() || invalidated[0].InvalidationReason != "desired hash changed" {
		t.Fatalf("invalidated = %+v", invalidated)
	}
	if registry.BaselineAuthorizes("engineering", "base/firewall", "hash-firewall", "nftables", true) {
		t.Fatal("invalidated baseline still authorized execution")
	}
}

func TestCanonicalBaselineRequiresCurrentContractProviderRevisionAndHash(t *testing.T) {
	const hash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	now := time.Date(2035, 4, 5, 6, 7, 8, 0, time.UTC)
	registry := NewRegistry(RegistryOptions{Now: func() time.Time { return now }, NewID: sequentialIDs("request", "rollout", "baseline")})
	plan := FleetPlan{
		Fleet: "engineering", ReleaseRef: "release", ArtifactDigest: "sha256:artifact", HashContractVersion: 1,
		Targets: []TargetEvidence{{EndpointID: "endpoint", Compatible: true, PreflightReady: true}},
		Resources: []ResourcePlan{{
			Address: "base/firewall", DesiredHash: hash, Risk: models.RiskConnectivity,
			Provider: "nftables", ProviderRevision: "firewall-v1", BaselineEligible: true,
		}},
	}
	requests, err := registry.CreateCanonicalChangeRequests(plan, []CanonicalResourceIdentity{{
		Address: "base/firewall", EffectiveHash: hash, Provider: "nftables",
		ProviderRevision: "firewall-v1", HashContractVersion: 1,
	}}, "creator")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.AuthorizeRollout(requests[0].ID, RolloutSpec{}, "approver", "CHG-BASELINE"); err != nil {
		t.Fatal(err)
	}
	baseline, err := registry.PromoteBaseline(requests[0].ID, "base/firewall", "operator")
	if err != nil {
		t.Fatal(err)
	}
	if baseline.HashContractVersion != 1 || baseline.ProviderRevision != "firewall-v1" {
		t.Fatalf("canonical baseline identity = %+v", baseline)
	}
	if !registry.BaselineAuthorizesCanonical("engineering", "base/firewall", hash, "nftables", "firewall-v1", 1, true) {
		t.Fatal("matching canonical baseline was not eligible")
	}
	if registry.BaselineAuthorizesCanonical("engineering", "base/firewall", hash, "nftables", "firewall-v2", 1, true) ||
		registry.BaselineAuthorizesCanonical("engineering", "base/firewall", hash, "nftables", "firewall-v1", 2, true) ||
		registry.BaselineAuthorizesCanonical("engineering", "base/firewall", "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "nftables", "firewall-v1", 1, true) {
		t.Fatal("baseline authorized a changed contract revision, hash contract, or desired hash")
	}
}

func sequentialIDs(ids ...string) func() string {
	index := 0
	return func() string {
		id := ids[index]
		index++
		return id
	}
}
