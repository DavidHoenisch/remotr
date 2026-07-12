package changecontrol

import (
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestBreakGlassAuthorizationIsBoundedAndAudited(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	registry := NewRegistry(RegistryOptions{
		Now: func() time.Time { return now }, NewID: sequentialIDs("break-glass"),
		CanBreakGlass: func(actorID, _ string, risk models.RiskClass) bool {
			return actorID == "operator" && risk == models.RiskConnectivity
		},
	})
	authorization, err := registry.CreateBreakGlass(BreakGlassSpec{
		Fleet: "fleet", EndpointIDs: []string{"endpoint"}, ResourceHashes: map[string]string{"network/firewall": "sha256:exact"},
		Risk: models.RiskConnectivity, Justification: "restore management", ExternalReference: "INC-42",
		Safeguards: BreakGlassSafeguards{SchemaValid: true, ProviderValid: true, RedactionEnabled: true, CurrentPreflightReady: true, RequiredRollbackReady: true},
	}, "operator", "")
	if err != nil {
		t.Fatal(err)
	}
	if authorization.AttemptLimit != 1 || authorization.ExpiresAt.Sub(now) != time.Hour || len(authorization.EndpointIDs) != 1 {
		t.Fatalf("unexpected defaults: %+v", authorization)
	}
	if authorization.ResourceHashes["network/firewall"] != "sha256:exact" || authorization.AuditHistory[0].Action != AuditBreakGlassCreated {
		t.Fatalf("authorization did not freeze hashes or audit creation: %+v", authorization)
	}
	if _, err := registry.UseBreakGlass(authorization.ID, "endpoint", map[string]string{"network/firewall": "sha256:other"}); err == nil {
		t.Fatal("mismatched resource hash was accepted")
	}
	used, err := registry.UseBreakGlass(authorization.ID, "endpoint", map[string]string{"network/firewall": "sha256:exact"})
	if err != nil || used.AuditHistory[len(used.AuditHistory)-1].Action != AuditBreakGlassUsed {
		t.Fatalf("used=%+v err=%v", used, err)
	}
	if _, err := registry.UseBreakGlass(authorization.ID, "endpoint", map[string]string{"network/firewall": "sha256:exact"}); err == nil {
		t.Fatal("second attempt was accepted")
	}
	revoked, err := registry.RevokeBreakGlass(authorization.ID, "operator")
	if err != nil || revoked.AuditHistory[len(revoked.AuditHistory)-1].Action != AuditBreakGlassRevoked {
		t.Fatalf("revoked=%+v err=%v", revoked, err)
	}
}

func TestBreakGlassCannotBypassSafeguardsOrFleetSecondOperator(t *testing.T) {
	registry := NewRegistry(RegistryOptions{CanBreakGlass: func(actorID, _ string, _ models.RiskClass) bool { return actorID == "one" || actorID == "two" }})
	valid := BreakGlassSpec{Fleet: "fleet", EndpointIDs: []string{"one", "two"}, FleetScope: true, ResourceHashes: map[string]string{"storage/disk": "hash"}, Risk: models.RiskDestructive, Justification: "incident", ExternalReference: "INC-9", Safeguards: BreakGlassSafeguards{SchemaValid: true, ProviderValid: true, RedactionEnabled: true, CurrentPreflightReady: true, RequiredRollbackReady: true, StableDeviceIdentity: true, IrreversibleApproved: true}}
	if _, err := registry.CreateBreakGlass(valid, "one", ""); err == nil {
		t.Fatal("fleet scope without second operator was accepted")
	}
	if _, err := registry.CreateBreakGlass(valid, "one", "one"); err == nil {
		t.Fatal("same operator counted twice")
	}
	invalid := valid
	invalid.Safeguards.StableDeviceIdentity = false
	if _, err := registry.CreateBreakGlass(invalid, "one", "two"); err == nil {
		t.Fatal("destructive request without stable identity was accepted")
	}
	if _, err := registry.CreateBreakGlass(valid, "one", "two"); err != nil {
		t.Fatal(err)
	}
}
