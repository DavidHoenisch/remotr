package configcompose

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/DavidHoenisch/remotr/internal/artifactrequirements"
	"github.com/DavidHoenisch/remotr/internal/artifactvariant"
	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
	"github.com/DavidHoenisch/remotr/internal/capabilitymatrix"
	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"gopkg.in/yaml.v3"
)

// RenderedArtifactVariant binds one bounded desired-state variant to its
// shared Fleet or endpoint-override source.
type RenderedArtifactVariant struct {
	TargetType   string
	TargetID     string
	ArtifactType string
	Variant      artifactvariant.Variant
}

// RenderAllArtifactVariants composes every shared target source. Cardinality
// is bounded by target count multiplied by the two declared schema versions.
func RenderAllArtifactVariants(repoRoot string) ([]RenderedArtifactVariant, error) {
	targets, err := ListCompositionTargets(repoRoot)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no fleet manifests found")
	}
	var rendered []RenderedArtifactVariant
	for _, target := range targets {
		variants, err := RenderManifestVariants(repoRoot, target.Manifest)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", target.Manifest, err)
		}
		targetType, targetID := "fleet", target.FleetName
		if target.EndpointID != "" {
			targetType, targetID = "endpoint", target.EndpointID
		}
		for _, variant := range variants {
			rendered = append(rendered, RenderedArtifactVariant{
				TargetType: targetType, TargetID: targetID, ArtifactType: "desired", Variant: variant,
			})
		}
	}
	return rendered, nil
}

// RenderFleetVariants composes the canonical schema-1 artifact and a schema-0
// compatibility artifact only when a schema-0 round trip preserves the exact
// canonical desired behavior.
func RenderFleetVariants(repoRoot, fleetName string) ([]artifactvariant.Variant, error) {
	if err := configrepo.ValidateFleetName(fleetName); err != nil {
		return nil, err
	}
	manifest, err := FindManifestInTree(repoRoot, filepath.Join("fleets", fleetName))
	if err != nil {
		return nil, fmt.Errorf("fleet %q: %w", fleetName, err)
	}
	return RenderManifestVariants(repoRoot, manifest)
}

// RenderEndpointVariants composes bounded variants for one endpoint override
// source. The result depends on authored desired state, not endpoint evidence.
func RenderEndpointVariants(repoRoot, endpointID string) ([]artifactvariant.Variant, error) {
	if err := configrepo.ValidateEndpointID(endpointID); err != nil {
		return nil, err
	}
	manifest, err := FindManifestInTree(repoRoot, filepath.Join("endpoints", endpointID))
	if err != nil {
		return nil, fmt.Errorf("endpoint %q: %w", endpointID, err)
	}
	return RenderManifestVariants(repoRoot, manifest)
}

// RenderManifestVariants emits at most the declared schema variants for one
// composed source. It never removes resources or fields to make a variant.
func RenderManifestVariants(repoRoot, manifestRel string) ([]artifactvariant.Variant, error) {
	manifestRel = normalizeRelPath(manifestRel)
	state, err := composeManifest(repoRoot, manifestRel)
	if err != nil {
		return nil, err
	}
	if err := configrepo.ValidateState(state, manifestRel); err != nil {
		return nil, fmt.Errorf("validate desired: %w", err)
	}
	schema1, err := resourceregistry.MarshalCanonical(state)
	if err != nil {
		return nil, err
	}
	sourceDigest := digestBytes(schema1)
	variants := make([]artifactvariant.Variant, 0, 2)
	schema1Variant, err := buildVariant(state, schema1, sourceDigest, 1)
	if err != nil {
		return nil, err
	}
	variants = append(variants, schema1Variant)

	schema0, lossless, err := marshalLosslessSchema0(state, schema1)
	if err != nil {
		return nil, err
	}
	if lossless {
		schema0Variant, err := buildVariant(state, schema0, sourceDigest, 0)
		if err != nil {
			return nil, err
		}
		variants = append(variants, schema0Variant)
	}
	return variants, nil
}

func buildVariant(state models.State, artifact []byte, sourceDigest string, schemaVersion int) (artifactvariant.Variant, error) {
	requirements, err := deriveRequirementSet(state, schemaVersion)
	if err != nil {
		return artifactvariant.Variant{}, err
	}
	requirementDigest, err := requirements.CanonicalDigest()
	if err != nil {
		return artifactvariant.Variant{}, err
	}
	return artifactvariant.Variant{
		Artifact: append([]byte(nil), artifact...), Digest: digestBytes(artifact),
		SourceDigest: sourceDigest, SchemaVersion: schemaVersion,
		Requirements: requirements, RequirementDigest: requirementDigest,
	}, nil
}

func deriveRequirementSet(state models.State, schemaVersion int) (artifactrequirements.Set, error) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		return artifactrequirements.Set{}, err
	}
	resources := make(map[string]artifactrequirements.Requirement)
	providers := make(map[string]artifactrequirements.Requirement)
	for configurationIndex := range state.Configurations {
		composed, err := registry.Resources(&state.Configurations[configurationIndex])
		if err != nil {
			return artifactrequirements.Set{}, err
		}
		for _, resource := range composed {
			resourceID := capabilitydoc.ResourceCapabilityID(string(resource.Kind()))
			resources[resourceID] = artifactrequirements.Requirement{ID: resourceID, Revision: resource.ProviderContractRevision()}
			for _, requirementID := range capabilitymatrix.Requirements(resource.Kind(), resource.Value()) {
				if len(requirementID) > len("provider:") && requirementID[:len("provider:")] == "provider:" {
					providers[requirementID] = artifactrequirements.Requirement{ID: requirementID, Revision: "1"}
				}
			}
		}
	}
	set := artifactrequirements.Set{Version: artifactrequirements.CurrentVersion, ArtifactSchemaVersion: schemaVersion}
	for _, requirement := range resources {
		set.ResourceCapabilities = append(set.ResourceCapabilities, requirement)
	}
	for _, requirement := range providers {
		set.ProviderCapabilities = append(set.ProviderCapabilities, requirement)
	}
	sort.Slice(set.ResourceCapabilities, func(i, j int) bool { return set.ResourceCapabilities[i].ID < set.ResourceCapabilities[j].ID })
	sort.Slice(set.ProviderCapabilities, func(i, j int) bool { return set.ProviderCapabilities[i].ID < set.ProviderCapabilities[j].ID })
	if err := set.Validate(); err != nil {
		return artifactrequirements.Set{}, err
	}
	return set, nil
}

func marshalLosslessSchema0(state models.State, canonicalSchema1 []byte) ([]byte, bool, error) {
	legacy := state
	legacy.SchemaVersion = 0
	legacy.Diagnostics = nil
	legacy.ResourceSources = nil
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(legacy); err != nil {
		return nil, false, err
	}
	if err := encoder.Close(); err != nil {
		return nil, false, err
	}
	roundTripped, err := models.ParseState(bytes.NewReader(output.Bytes()))
	if err != nil {
		return nil, false, nil
	}
	recanonical, err := resourceregistry.MarshalCanonical(roundTripped)
	if err != nil {
		return nil, false, nil
	}
	return output.Bytes(), bytes.Equal(recanonical, canonicalSchema1), nil
}
