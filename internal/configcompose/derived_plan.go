package configcompose

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/effectivehash"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/providercontract"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

// DerivedFleetPlan carries the server-authored review plan and the independent
// composition identities required by Change-control admission.
type DerivedFleetPlan struct {
	Plan              changecontrol.FleetPlan
	TrustedIdentities []changecontrol.CanonicalResourceIdentity
}

// DeriveFleetPlan builds authoritative plan evidence from the same composed,
// registered resources and provider selections used for canonical hashes.
func DeriveFleetPlan(ctx context.Context, fleet, releaseRef, artifactDigest string, state models.State, selections map[string]ProviderSelection, resolver secrets.Resolver) (DerivedFleetPlan, error) {
	if strings.TrimSpace(fleet) == "" || fleet != strings.TrimSpace(fleet) {
		return DerivedFleetPlan{}, fmt.Errorf("fleet is required")
	}
	if strings.TrimSpace(releaseRef) == "" || releaseRef != strings.TrimSpace(releaseRef) {
		return DerivedFleetPlan{}, fmt.Errorf("release ref is required")
	}
	effective, err := EffectiveResources(ctx, state, selections, artifactDigest, resolver)
	if err != nil {
		return DerivedFleetPlan{}, err
	}
	identities := make(map[string]EffectiveResource, len(effective))
	trusted := make([]changecontrol.CanonicalResourceIdentity, 0, len(effective))
	for _, identity := range effective {
		identities[identity.Address] = identity
		trusted = append(trusted, changecontrol.CanonicalResourceIdentity{
			Address: identity.Address, EffectiveHash: identity.EffectiveHash,
			Provider: identity.ProviderID, ProviderRevision: identity.ProviderRevision,
			HashContractVersion: identity.HashContractVersion,
		})
	}

	registry, err := resourceregistry.NewDefault()
	if err != nil {
		return DerivedFleetPlan{}, err
	}
	resources := make([]changecontrol.ResourcePlan, 0, len(effective))
	for configurationIndex := range state.Configurations {
		configuration := &state.Configurations[configurationIndex]
		registered, err := registry.Resources(configuration)
		if err != nil {
			return DerivedFleetPlan{}, err
		}
		for _, resource := range registered {
			address := models.ResourceAddress(configuration.Name, resource.Name())
			identity, ok := identities[address]
			if !ok {
				return DerivedFleetPlan{}, fmt.Errorf("resource %q has no canonical identity", address)
			}
			descriptor, err := resource.PlanDescriptor(identity.ProviderID)
			if err != nil {
				return DerivedFleetPlan{}, fmt.Errorf("resource %q: %w", address, err)
			}
			metadata := resource.Metadata()
			resources = append(resources, changecontrol.ResourcePlan{
				Address: address, DesiredHash: identity.EffectiveHash,
				Risk:     metadata.EffectiveRisk(resource.DefaultRisk()),
				Provider: identity.ProviderID, ProviderRevision: identity.ProviderRevision,
				AuthorizationGroup: metadata.AuthorizationGroup,
				DependsOn:          append([]string(nil), metadata.DependsOn...),
				ActivationTargets:  planActivationTargets(descriptor.ActivationTargets),
				PredictedEffects:   planEffects(descriptor.Effects),
				RollbackClass:      string(descriptor.RollbackClass), BaselineEligible: descriptor.BaselineEligible,
			})
		}
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Address < resources[j].Address })
	return DerivedFleetPlan{
		Plan: changecontrol.FleetPlan{
			Fleet: fleet, ReleaseRef: releaseRef, ArtifactDigest: artifactDigest,
			HashContractVersion: effectivehash.SchemaVersion, Resources: resources,
		},
		TrustedIdentities: trusted,
	}, nil
}

func planEffects(effects []providercontract.PlanEffect) []changecontrol.PredictedEffect {
	out := make([]changecontrol.PredictedEffect, len(effects))
	for index, effect := range effects {
		out[index] = changecontrol.PredictedEffect{
			Code: changecontrol.EffectCode(effect.Code), Details: effect.Details.Clone(),
		}
	}
	return out
}

func planActivationTargets(targets []providercontract.ActivationTarget) []string {
	out := make([]string, len(targets))
	for index, target := range targets {
		out[index] = string(target.Kind)
		if target.Target != "" {
			out[index] += ":" + target.Target
		}
	}
	return out
}
