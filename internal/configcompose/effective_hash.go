package configcompose

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/effectivehash"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

// ProviderSelection is trusted provider identity chosen for one composed
// resource after endpoint capability evaluation.
type ProviderSelection struct {
	ID string
}

// EffectiveResource is the canonical identity carried from composition into
// plans, leases, agent verification, and reports.
type EffectiveResource struct {
	Address             string
	Kind                models.ResourceKind
	ProviderID          string
	ProviderRevision    string
	HashContractVersion int
	EffectiveHash       string
}

// EffectiveResources derives one canonical identity for every composed,
// registered resource. Provider selection is an explicit trusted input and
// cannot be inferred from caller-authored plan data.
func EffectiveResources(ctx context.Context, state models.State, selections map[string]ProviderSelection, artifactDigest string, resolver secrets.Resolver) ([]EffectiveResource, error) {
	if strings.TrimSpace(artifactDigest) == "" || artifactDigest != strings.TrimSpace(artifactDigest) {
		return nil, fmt.Errorf("artifact digest is required")
	}
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	var output []EffectiveResource
	for configurationIndex := range state.Configurations {
		configuration := &state.Configurations[configurationIndex]
		resources, err := registry.Resources(configuration)
		if err != nil {
			return nil, err
		}
		for _, resource := range resources {
			address := models.ResourceAddress(configuration.Name, resource.Name())
			selection, ok := selections[address]
			if !ok || strings.TrimSpace(selection.ID) == "" || selection.ID != strings.TrimSpace(selection.ID) {
				return nil, fmt.Errorf("resource %q requires a trusted provider selection", address)
			}
			source, ok := state.ResourceSources[address]
			if !ok {
				return nil, fmt.Errorf("resource %q has no schema-1 source presence evidence", address)
			}
			resource, err = resource.BindSource(&source)
			if err != nil {
				return nil, fmt.Errorf("resource %q source: %w", address, err)
			}
			hash, err := resource.ResolveEffectiveHash(ctx, address, selection.ID, artifactDigest, resolver)
			if err != nil {
				return nil, fmt.Errorf("resource %q effective hash: %w", address, err)
			}
			output = append(output, EffectiveResource{
				Address: address, Kind: resource.Kind(), ProviderID: selection.ID,
				ProviderRevision:    resource.ProviderContractRevision(),
				HashContractVersion: effectivehash.SchemaVersion, EffectiveHash: hash,
			})
			seen[address] = struct{}{}
		}
	}
	for address := range selections {
		if _, ok := seen[address]; !ok {
			return nil, fmt.Errorf("provider selection references unknown composed resource %q", address)
		}
	}
	sort.Slice(output, func(i, j int) bool { return output[i].Address < output[j].Address })
	return output, nil
}
