package changecontrol

import (
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestBreakGlassAuthorizationIsBoundedAndAudited(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	registry := NewRegistry(RegistryOptions{
		Now: func() time.Time { return now }, NewID: sequentialIDs("request", "break-glass"),
		CanBreakGlass: func(actorID, _ string, risk models.RiskClass) bool {
			return actorID == "operator" && risk == models.RiskConnectivity
		},
	})
	request := createCanonicalBreakGlassRequest(t, registry, models.RiskConnectivity, "endpoint")
	authorization, err := registry.CreateBreakGlass(BreakGlassSpec{
		ChangeRequestID: request.ID, EndpointIDs: []string{"endpoint"},
		Justification: "restore management", ExternalReference: "INC-42",
	}, "operator", "")
	if err != nil {
		t.Fatal(err)
	}
	if authorization.AttemptLimit != 1 || authorization.ExpiresAt.Sub(now) != time.Hour || len(authorization.EndpointIDs) != 1 {
		t.Fatalf("unexpected defaults: %+v", authorization)
	}
	if authorization.ResourceHashes["network/firewall"] != canonicalBreakGlassHash || authorization.AuditHistory[0].Action != AuditBreakGlassCreated {
		t.Fatalf("authorization did not freeze hashes or audit creation: %+v", authorization)
	}
	if _, err := registry.UseBreakGlass(authorization.ID, PreflightReport{ChangeRequestID: request.ID, EndpointID: "endpoint", Ready: true, ResourceHashes: map[string]string{"network/firewall": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}); err == nil {
		t.Fatal("mismatched resource hash was accepted")
	}
	used, err := registry.UseBreakGlass(authorization.ID, PreflightReport{ChangeRequestID: request.ID, EndpointID: "endpoint", Ready: true, ResourceHashes: map[string]string{"network/firewall": canonicalBreakGlassHash}})
	if err != nil || used.AuditHistory[len(used.AuditHistory)-1].Action != AuditBreakGlassUsed {
		t.Fatalf("used=%+v err=%v", used, err)
	}
	if _, err := registry.UseBreakGlass(authorization.ID, PreflightReport{ChangeRequestID: request.ID, EndpointID: "endpoint", Ready: true, ResourceHashes: map[string]string{"network/firewall": canonicalBreakGlassHash}}); err == nil {
		t.Fatal("second attempt was accepted")
	}
	revoked, err := registry.RevokeBreakGlass(authorization.ID, "operator")
	if err != nil || revoked.AuditHistory[len(revoked.AuditHistory)-1].Action != AuditBreakGlassRevoked {
		t.Fatalf("revoked=%+v err=%v", revoked, err)
	}
}

func TestBreakGlassFleetAndDestructiveScopeRequireSecondOperator(t *testing.T) {
	registry := NewRegistry(RegistryOptions{NewID: sequentialIDs("request", "break-glass"), CanBreakGlass: func(actorID, _ string, _ models.RiskClass) bool { return actorID == "one" || actorID == "two" }})
	request := createCanonicalBreakGlassRequest(t, registry, models.RiskDestructive, "one", "two")
	valid := BreakGlassSpec{ChangeRequestID: request.ID, EndpointIDs: []string{"one", "two"}, FleetScope: true, Justification: "incident", ExternalReference: "INC-9"}
	if _, err := registry.CreateBreakGlass(valid, "one", ""); err == nil {
		t.Fatal("fleet scope without second operator was accepted")
	}
	if _, err := registry.CreateBreakGlass(valid, "one", "one"); err == nil {
		t.Fatal("same operator counted twice")
	}
	if _, err := registry.CreateBreakGlass(valid, "one", "two"); err != nil {
		t.Fatal(err)
	}
}

const canonicalBreakGlassHash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func createCanonicalBreakGlassRequest(t *testing.T, registry *Registry, risk models.RiskClass, endpointIDs ...string) ChangeRequest {
	t.Helper()
	targets := make([]TargetEvidence, len(endpointIDs))
	for index, endpointID := range endpointIDs {
		targets[index] = TargetEvidence{
			EndpointID: endpointID, Compatible: true, PreflightReady: true,
			ResourcePreflights: []ResourcePreflightEvidence{{Address: "network/firewall", Ready: true}},
		}
	}
	plan := FleetPlan{
		Fleet: "fleet", ReleaseRef: "release", ArtifactDigest: "sha256:artifact", HashContractVersion: 1,
		Targets: targets,
		Resources: []ResourcePlan{{
			Address: "network/firewall", DesiredHash: canonicalBreakGlassHash, Risk: risk,
			Provider: "firewall", ProviderRevision: "firewall-v1", RollbackClass: "transactional",
		}},
	}
	requests, err := registry.CreateCanonicalChangeRequests(plan, []CanonicalResourceIdentity{{
		Address: "network/firewall", EffectiveHash: canonicalBreakGlassHash, Provider: "firewall",
		ProviderRevision: "firewall-v1", HashContractVersion: 1,
	}}, "planner")
	if err != nil || len(requests) != 1 {
		t.Fatalf("canonical request = %+v, %v", requests, err)
	}
	return requests[0]
}

func TestBreakGlassCannotBypassCanonicalRequestSafetyEvidence(t *testing.T) {
	const currentHash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	registry := NewRegistry(RegistryOptions{
		NewID: sequentialIDs("request", "break-glass"),
		CanBreakGlass: func(actorID, fleet string, risk models.RiskClass) bool {
			return actorID == "operator" && fleet == "fleet" && risk == models.RiskConnectivity
		},
	})
	plan := FleetPlan{
		Fleet: "fleet", ReleaseRef: "release", ArtifactDigest: "sha256:artifact", HashContractVersion: 1,
		Targets: []TargetEvidence{
			{EndpointID: "blocked", Compatible: true, PreflightReason: "rollback_reservation_failed", ResourcePreflights: []ResourcePreflightEvidence{{Address: "network/firewall", Reason: "rollback_reservation_failed"}}},
			{EndpointID: "ready", Compatible: true, PreflightReady: true, ResourcePreflights: []ResourcePreflightEvidence{{Address: "network/firewall", Ready: true}}},
		},
		Resources: []ResourcePlan{{
			Address: "network/firewall", DesiredHash: currentHash, Risk: models.RiskConnectivity,
			Provider: "firewall", ProviderRevision: "firewall-v1", RollbackClass: "transactional",
		}},
	}
	requests, err := registry.CreateCanonicalChangeRequests(plan, []CanonicalResourceIdentity{{
		Address: "network/firewall", EffectiveHash: currentHash, Provider: "firewall",
		ProviderRevision: "firewall-v1", HashContractVersion: 1,
	}}, "planner")
	if err != nil {
		t.Fatal(err)
	}
	spec := BreakGlassSpec{
		ChangeRequestID: requests[0].ID, EndpointIDs: []string{"blocked"},
		Justification: "restore management", ExternalReference: "INC-42",
	}
	if _, err := registry.CreateBreakGlass(spec, "operator", ""); err == nil {
		t.Fatal("rollback-blocked frozen target was accepted")
	}
	spec.EndpointIDs = []string{"ready"}
	authorization, err := registry.CreateBreakGlass(spec, "operator", "")
	if err != nil {
		t.Fatal(err)
	}
	if authorization.ChangeRequestID != requests[0].ID || authorization.Fleet != "fleet" || authorization.Risk != models.RiskConnectivity || authorization.ResourceHashes["network/firewall"] != currentHash {
		t.Fatalf("derived break glass = %+v", authorization)
	}
	if _, err := registry.UseBreakGlass(authorization.ID, PreflightReport{
		ChangeRequestID: requests[0].ID, EndpointID: "ready", Ready: false,
		ResourceHashes: map[string]string{"network/firewall": currentHash},
	}); err == nil {
		t.Fatal("fresh blocked preflight was accepted")
	}
	if _, err := registry.UseBreakGlass(authorization.ID, PreflightReport{
		ChangeRequestID: requests[0].ID, EndpointID: "ready", Ready: true,
		ResourceHashes: map[string]string{"network/firewall": currentHash},
	}); err != nil {
		t.Fatal(err)
	}
}
