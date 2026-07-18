package changecontrol

import (
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestRegistryCreateChangeRequestsGroupsAndFreezesFleetPlan(t *testing.T) {
	now := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	nextID := 0
	registry := NewRegistry(RegistryOptions{
		Now: func() time.Time { return now },
		NewID: func() string {
			nextID++
			return "change-" + string(rune('0'+nextID))
		},
	})
	plan := FleetPlan{
		Fleet:          "engineering",
		ReleaseRef:     "release-abc",
		ArtifactDigest: "sha256:artifact",
		Targets: []TargetEvidence{
			{EndpointID: "endpoint-a", Compatible: true, PreflightReady: true},
			{EndpointID: "endpoint-b", Compatible: false, PreflightReason: "provider unavailable"},
		},
		Resources: []ResourcePlan{
			{Address: "base/package", DesiredHash: "sha256:pkg", Risk: models.RiskNormal, Provider: "apt"},
			{Address: "base/dns", DesiredHash: "sha256:dns", Risk: models.RiskConnectivity, Provider: "networkmanager", AuthorizationGroup: "network-transition", DependsOn: []string{"base/package"}, PredictedEffects: []PredictedEffect{{Code: EffectNetworkDNSReplace}}, RollbackClass: "transactional"},
			{Address: "base/route", DesiredHash: "sha256:route", Risk: models.RiskConnectivity, Provider: "networkmanager", AuthorizationGroup: "network-transition", DependsOn: []string{"base/dns"}, PredictedEffects: []PredictedEffect{{Code: EffectDefaultRouteReplace}}, RollbackClass: "transactional"},
			{Address: "base/sudo", DesiredHash: "sha256:sudo", Risk: models.RiskAccess, Provider: "sudo", PredictedEffects: []PredictedEffect{{Code: EffectSudoPolicyReplace}}, RollbackClass: "best_effort"},
		},
	}

	requests, err := registry.CreateChangeRequests(plan, "operator-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %+v", requests)
	}
	byGroup := map[string]ChangeRequest{}
	for _, request := range requests {
		byGroup[request.AuthorizationGroup] = request
		if request.Fleet != "engineering" || request.ReleaseRef != "release-abc" || request.ArtifactDigest != "sha256:artifact" {
			t.Fatalf("request identity = %+v", request)
		}
		if request.AuthorizationState != AuthorizationPending || len(request.AuditHistory) != 1 || request.AuditHistory[0].Action != AuditCreated || request.AuditHistory[0].ActorID != "operator-1" {
			t.Fatalf("request state/audit = %+v", request)
		}
		if len(request.FrozenTargets) != 2 || request.FrozenTargets[1].EndpointID != "endpoint-b" || request.FrozenTargets[1].PreflightReason != "provider unavailable" {
			t.Fatalf("frozen targets = %+v", request.FrozenTargets)
		}
	}

	network := byGroup["network-transition"]
	if len(network.Resources) != 3 || network.ResourceHashes["base/package"] != "sha256:pkg" || network.ResourceHashes["base/dns"] != "sha256:dns" || network.ResourceHashes["base/route"] != "sha256:route" {
		t.Fatalf("network request = %+v", network)
	}
	if network.Resources[0].Risk != models.RiskNormal {
		t.Fatalf("normal prerequisite inherited risk: %+v", network.Resources[0])
	}
	if network.Risk != models.RiskConnectivity {
		t.Fatalf("network strictest risk = %q", network.Risk)
	}

	sudo := byGroup["component:base/sudo"]
	if len(sudo.Resources) != 1 || sudo.Resources[0].Address != "base/sudo" || sudo.Risk != models.RiskAccess {
		t.Fatalf("sudo request = %+v", sudo)
	}

	plan.Targets[0].EndpointID = "mutated"
	plan.Resources[1].DesiredHash = "mutated"
	stored, ok := registry.Get(network.ID)
	if !ok || stored.FrozenTargets[0].EndpointID != "endpoint-a" || stored.ResourceHashes["base/dns"] != "sha256:dns" {
		t.Fatalf("stored request was not frozen: %+v", stored)
	}
}

func TestCanonicalChangeRequestBoundaryRejectsCallerHashMismatch(t *testing.T) {
	const currentHash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const callerHash = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	plan := FleetPlan{
		Fleet: "engineering", ReleaseRef: "release", ArtifactDigest: "sha256:artifact", HashContractVersion: 1,
		Targets: []TargetEvidence{{EndpointID: "endpoint", Compatible: true, PreflightReady: true}},
		Resources: []ResourcePlan{{
			Address: "base/firewall", DesiredHash: callerHash, Risk: models.RiskConnectivity,
			Provider: "nftables", ProviderRevision: "firewall-v1",
		}},
	}
	registry := NewRegistry(RegistryOptions{NewID: sequentialIDs("request")})
	if _, err := registry.CreateChangeRequests(plan, "caller"); err == nil {
		t.Fatal("legacy Change-request boundary accepted a caller claiming canonical authority")
	}
	trusted := []CanonicalResourceIdentity{{
		Address: "base/firewall", EffectiveHash: currentHash, Provider: "nftables",
		ProviderRevision: "firewall-v1", HashContractVersion: 1,
	}}
	if _, err := registry.CreateCanonicalChangeRequests(plan, trusted, "caller"); err == nil {
		t.Fatal("canonical Change-request boundary accepted a conflicting caller hash")
	}
	plan.Resources[0].DesiredHash = currentHash
	requests, err := registry.CreateCanonicalChangeRequests(plan, trusted, "server-composition")
	if err != nil || len(requests) != 1 || requests[0].ResourceHashes["base/firewall"] != currentHash {
		t.Fatalf("canonical Change request = %+v err=%v", requests, err)
	}
}

// OS-AEC-032: a mixed-risk explicit group inherits its strictest policy,
// including the sensitive tier between normal and connectivity risk.
func TestRegistryMixedRiskGroupUsesStrictestAuthorizationPolicy(t *testing.T) {
	registry := NewRegistry(RegistryOptions{NewID: sequentialIDs("request")})
	requests, err := registry.CreateChangeRequests(FleetPlan{
		Fleet: "engineering", ReleaseRef: "release", ArtifactDigest: "sha256:artifact",
		Targets: []TargetEvidence{{EndpointID: "endpoint", Compatible: true, PreflightReady: true}},
		Resources: []ResourcePlan{
			{Address: "base/credential", DesiredHash: "sha256:credential", Risk: models.RiskSensitive, Provider: "secret", AuthorizationGroup: "transition"},
			{Address: "base/firewall", DesiredHash: "sha256:firewall", Risk: models.RiskConnectivity, Provider: "nftables", AuthorizationGroup: "transition"},
		},
	}, "operator")
	if err != nil || len(requests) != 1 || requests[0].Risk != models.RiskConnectivity {
		t.Fatalf("CreateChangeRequests() = %+v, err=%v", requests, err)
	}
}
