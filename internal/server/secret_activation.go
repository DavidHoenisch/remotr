package server

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

// SecretActivationCoordinator creates Change requests for activation uses and
// gates active-version resolution on their rollout authorization.
type SecretActivationCoordinator struct {
	changes *changecontrol.Registry
	now     func() time.Time
}

func NewSecretActivationCoordinator(changes *changecontrol.Registry) *SecretActivationCoordinator {
	return &SecretActivationCoordinator{changes: changes, now: func() time.Time { return time.Now().UTC() }}
}

func (c *SecretActivationCoordinator) CreateActivationRollouts(_ context.Context, plan secrets.ActivationPlan) ([]secrets.RolloutBinding, error) {
	bindings := make([]secrets.RolloutBinding, len(plan.Uses))
	byFleet := make(map[string][]int)
	for i, use := range plan.Uses {
		bindings[i] = secrets.RolloutBinding{
			Fleet: use.Fleet, ResourceAddress: use.ResourceAddress, Purpose: use.Purpose,
			Risk: use.Risk, EffectiveHash: use.EffectiveHash,
		}
		if use.Risk.RequiresPreflight() {
			byFleet[use.Fleet] = append(byFleet[use.Fleet], i)
		}
	}
	if len(byFleet) > 0 && (c == nil || c.changes == nil) {
		return nil, fmt.Errorf("Change control is required for high-risk secret activation")
	}
	fleets := make([]string, 0, len(byFleet))
	for fleet := range byFleet {
		fleets = append(fleets, fleet)
	}
	sort.Strings(fleets)
	for _, fleet := range fleets {
		indexes := byFleet[fleet]
		first := plan.Uses[indexes[0]]
		fleetPlan := changecontrol.FleetPlan{Fleet: fleet, ReleaseRef: first.ReleaseRef, ArtifactDigest: first.ArtifactDigest}
		for _, endpointID := range first.EndpointIDs {
			fleetPlan.Targets = append(fleetPlan.Targets, changecontrol.TargetEvidence{EndpointID: endpointID, Compatible: true})
		}
		for _, index := range indexes {
			use := plan.Uses[index]
			if use.ReleaseRef != fleetPlan.ReleaseRef || use.ArtifactDigest != fleetPlan.ArtifactDigest {
				return nil, fmt.Errorf("activation uses for fleet %q do not share an active artifact", fleet)
			}
			fleetPlan.Resources = append(fleetPlan.Resources, changecontrol.ResourcePlan{
				Address: use.ResourceAddress, DesiredHash: use.EffectiveHash, Risk: use.Risk, Provider: use.Provider,
				AuthorizationGroup: "secret-activation:" + plan.Name,
				PredictedEffects: []changecontrol.PredictedEffect{{
					Code: changecontrol.EffectSecretVersionActivate,
					Details: executor.SafeSummary{Fields: []executor.SafeField{
						{Path: "secret", Sensitivity: executor.SafeSecret, Projection: executor.SafeReference, Text: plan.Name},
						{Path: "version", Sensitivity: executor.SafeSecret, Projection: executor.SafeReference, Text: plan.Version},
					}},
				}},
				RollbackClass: secretActivationRollbackClass(use.Risk),
			})
		}
		requests, err := c.changes.CreateChangeRequests(fleetPlan, plan.ActorID)
		if err != nil {
			return nil, err
		}
		for _, request := range requests {
			for _, resource := range request.Resources {
				for _, index := range indexes {
					if bindings[index].ResourceAddress == resource.Address {
						bindings[index].ChangeRequestID = request.ID
					}
				}
			}
		}
	}
	return bindings, nil
}

func (c *SecretActivationCoordinator) RolloutActive(_ context.Context, changeRequestID string) bool {
	return c != nil && c.changes != nil && c.changes.RolloutActive(changeRequestID, c.now())
}

func secretActivationRollbackClass(risk models.RiskClass) string {
	if risk == models.RiskConnectivity {
		return "transactional"
	}
	return "best_effort"
}

var (
	_ secrets.ActivationPlanner = (*SecretActivationCoordinator)(nil)
	_ secrets.RolloutGate       = (*SecretActivationCoordinator)(nil)
)
