package server

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/configcompose"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/secretref"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

// SecretActivationCoordinator creates Change requests for activation uses and
// gates active-version resolution on their rollout authorization.
type SecretActivationCoordinator struct {
	changes *changecontrol.Registry
	plans   *ChangePlanDeriver
	now     func() time.Time
}

func NewSecretActivationCoordinator(changes *changecontrol.Registry, plans ...*ChangePlanDeriver) *SecretActivationCoordinator {
	coordinator := &SecretActivationCoordinator{changes: changes, now: func() time.Time { return time.Now().UTC() }}
	if len(plans) > 0 {
		coordinator.plans = plans[0]
	}
	return coordinator
}

func (c *SecretActivationCoordinator) CreateActivationRollouts(ctx context.Context, plan secrets.ActivationPlan) ([]secrets.RolloutBinding, error) {
	bindings := make([]secrets.RolloutBinding, len(plan.Uses))
	byFleet := make(map[string][]int)
	for i, use := range plan.Uses {
		bindings[i] = secrets.RolloutBinding{
			Fleet: use.Fleet, ResourceAddress: use.ResourceAddress, Purpose: use.Purpose,
			Risk: use.Risk,
		}
		byFleet[use.Fleet] = append(byFleet[use.Fleet], i)
	}
	if len(byFleet) > 0 && (c == nil || c.changes == nil || c.plans == nil) {
		return nil, fmt.Errorf("canonical rollout planning is required for active secret consumers")
	}
	fleets := make([]string, 0, len(byFleet))
	for fleet := range byFleet {
		fleets = append(fleets, fleet)
	}
	sort.Strings(fleets)
	canonicalPlans := make([]changecontrol.CanonicalFleetPlan, 0, len(fleets))
	for _, fleet := range fleets {
		indexes := byFleet[fleet]
		first := plan.Uses[indexes[0]]
		changed := make(map[string]struct{}, len(indexes))
		for _, index := range indexes {
			use := plan.Uses[index]
			if use.ReleaseRef != first.ReleaseRef || use.ArtifactDigest != first.ArtifactDigest {
				return nil, fmt.Errorf("activation uses for fleet %q do not share an active artifact", fleet)
			}
			changed[use.ResourceAddress] = struct{}{}
		}
		resolver := activationIdentityResolver{base: c.plans.Secrets, plan: plan, uses: activationUseKeys(plan.Uses, indexes)}
		derived, err := c.plans.derive(ctx, fleet, first.ReleaseRef, resolver, changed)
		if err != nil {
			return nil, err
		}
		if derived.Plan.ArtifactDigest != first.ArtifactDigest {
			return nil, fmt.Errorf("activation uses for fleet %q do not match the current composed artifact", fleet)
		}
		identities := make(map[string]changecontrol.CanonicalResourceIdentity, len(derived.TrustedIdentities))
		for _, identity := range derived.TrustedIdentities {
			identities[identity.Address] = identity
		}
		for _, index := range indexes {
			identity, ok := identities[bindings[index].ResourceAddress]
			if !ok {
				return nil, fmt.Errorf("activation resource %q lacks canonical identity", bindings[index].ResourceAddress)
			}
			bindings[index].EffectiveHash = identity.EffectiveHash
		}
		fleetPlan, trusted, err := activationFleetPlan(derived, plan, indexes)
		if err != nil {
			return nil, err
		}
		canonicalPlans = append(canonicalPlans, changecontrol.CanonicalFleetPlan{Plan: fleetPlan, Trusted: trusted})
	}
	requests, err := c.changes.CreateCanonicalChangeRequestBatch(canonicalPlans, plan.ActorID)
	if err != nil {
		return nil, err
	}
	for _, request := range requests {
		for _, resource := range request.Resources {
			for index := range bindings {
				if bindings[index].Fleet == request.Fleet && bindings[index].ResourceAddress == resource.Address {
					bindings[index].ChangeRequestID = request.ID
					bindings[index].EffectiveHash = resource.DesiredHash
				}
			}
		}
	}
	return bindings, nil
}

type activationIdentityResolver struct {
	base secrets.Resolver
	plan secrets.ActivationPlan
	uses map[string]struct{}
}

func (r activationIdentityResolver) Resolve(ctx context.Context, request secrets.ResolveRequest) (secrets.Resolved, error) {
	reference, err := secretref.ParseSelected(request.Reference)
	if err != nil {
		return secrets.Resolved{}, err
	}
	if reference.Provider == secrets.ProviderRemotr && reference.FollowsActive() && reference.Name == r.plan.Name {
		if _, ok := r.uses[activationUseKey(request.ResourceAddress, request.Purpose)]; !ok {
			return secrets.Resolved{}, fmt.Errorf("active secret use is absent from the activation plan")
		}
		return secrets.Resolved{Provider: secrets.ProviderRemotr, Version: r.plan.Version, ActivationGeneration: r.plan.Generation}, nil
	}
	if r.base == nil {
		return secrets.Resolved{}, fmt.Errorf("current secret identity resolver is unavailable")
	}
	return r.base.Resolve(ctx, request)
}

func activationUseKeys(uses []secrets.ActivationUse, indexes []int) map[string]struct{} {
	keys := make(map[string]struct{}, len(indexes))
	for _, index := range indexes {
		keys[activationUseKey(uses[index].ResourceAddress, uses[index].Purpose)] = struct{}{}
	}
	return keys
}

func activationUseKey(address, purpose string) string {
	return fmt.Sprintf("%d:%s%s", len(address), address, purpose)
}

func activationFleetPlan(derived configcompose.DerivedFleetPlan, plan secrets.ActivationPlan, indexes []int) (changecontrol.FleetPlan, []changecontrol.CanonicalResourceIdentity, error) {
	resources := make(map[string]changecontrol.ResourcePlan, len(derived.Plan.Resources))
	for _, resource := range derived.Plan.Resources {
		resources[resource.Address] = resource
	}
	selected := make(map[string]struct{}, len(indexes))
	usesByAddress := make(map[string][]secrets.ActivationUse, len(indexes))
	for _, index := range indexes {
		use := plan.Uses[index]
		resource, ok := resources[use.ResourceAddress]
		if !ok || resource.Risk != use.Risk {
			return changecontrol.FleetPlan{}, nil, fmt.Errorf("activation resource %q does not match composed plan evidence", use.ResourceAddress)
		}
		selected[use.ResourceAddress] = struct{}{}
		usesByAddress[use.ResourceAddress] = append(usesByAddress[use.ResourceAddress], use)
	}
	for changed := true; changed; {
		changed = false
		for address := range selected {
			for _, dependency := range resources[address].DependsOn {
				if _, exists := selected[dependency]; !exists {
					selected[dependency] = struct{}{}
					changed = true
				}
			}
		}
	}
	fleetPlan := derived.Plan
	fleetPlan.Resources = nil
	for _, resource := range derived.Plan.Resources {
		if _, ok := selected[resource.Address]; !ok {
			continue
		}
		if len(usesByAddress[resource.Address]) > 0 {
			resource.AuthorizationGroup = "secret-activation:" + plan.Name
			resource.PredictedEffects = append(resource.PredictedEffects, changecontrol.PredictedEffect{
				Code: changecontrol.EffectSecretVersionActivate,
				Details: executor.SafeSummary{Fields: []executor.SafeField{
					{Path: "secret", Sensitivity: executor.SafeSecret, Projection: executor.SafeReference, Text: plan.Name},
					{Path: "version", Sensitivity: executor.SafeSecret, Projection: executor.SafeReference, Text: plan.Version},
				}},
			})
		}
		fleetPlan.Resources = append(fleetPlan.Resources, resource)
	}
	trusted := make([]changecontrol.CanonicalResourceIdentity, 0, len(selected))
	for _, identity := range derived.TrustedIdentities {
		if _, ok := selected[identity.Address]; ok {
			trusted = append(trusted, identity)
		}
	}
	return fleetPlan, trusted, nil
}

func (c *SecretActivationCoordinator) RolloutActive(_ context.Context, changeRequestID string) bool {
	return c != nil && c.changes != nil && c.changes.RolloutActive(changeRequestID, c.now())
}

var (
	_ secrets.ActivationPlanner = (*SecretActivationCoordinator)(nil)
	_ secrets.RolloutGate       = (*SecretActivationCoordinator)(nil)
)
