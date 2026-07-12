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
			{Address: "base/dns", DesiredHash: "sha256:dns", Risk: models.RiskConnectivity, Provider: "networkmanager", AuthorizationGroup: "network-transition", DependsOn: []string{"base/package"}, PredictedEffects: []string{"replace DNS servers"}, RollbackClass: "transactional"},
			{Address: "base/route", DesiredHash: "sha256:route", Risk: models.RiskConnectivity, Provider: "networkmanager", AuthorizationGroup: "network-transition", DependsOn: []string{"base/dns"}, PredictedEffects: []string{"replace default route"}, RollbackClass: "transactional"},
			{Address: "base/sudo", DesiredHash: "sha256:sudo", Risk: models.RiskAccess, Provider: "sudo", PredictedEffects: []string{"replace sudo fragment"}, RollbackClass: "best_effort"},
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
