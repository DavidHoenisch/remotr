package configcompose

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/artifactrequirements"
	"github.com/DavidHoenisch/remotr/internal/artifactvariant"
	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
	"github.com/DavidHoenisch/remotr/internal/capabilitymatrix"
	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"gopkg.in/yaml.v3"
)

const MaxTargetPredicates = 64

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
	predicates, err := targetPredicates(state)
	if err != nil {
		return nil, err
	}
	variants := make([]artifactvariant.Variant, 0, len(predicates)*2)
	for _, predicate := range predicates {
		schema1Variant, err := buildVariant(state, schema1, sourceDigest, 1, predicate)
		if err != nil {
			return nil, err
		}
		variants = append(variants, schema1Variant)
	}

	schema0, lossless, err := marshalLosslessSchema0(state, schema1)
	if err != nil {
		return nil, err
	}
	if lossless {
		for _, predicate := range predicates {
			schema0Variant, err := buildVariant(state, schema0, sourceDigest, 0, predicate)
			if err != nil {
				return nil, err
			}
			variants = append(variants, schema0Variant)
		}
	}
	return variants, nil
}

func buildVariant(state models.State, artifact []byte, sourceDigest string, schemaVersion int, target *artifactrequirements.TargetPredicate) (artifactvariant.Variant, error) {
	requirements, err := deriveRequirementSet(state, schemaVersion, target)
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

func deriveRequirementSet(state models.State, schemaVersion int, target *artifactrequirements.TargetPredicate) (artifactrequirements.Set, error) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		return artifactrequirements.Set{}, err
	}
	resources := make(map[string]artifactrequirements.Requirement)
	providers := make(map[string]artifactrequirements.Requirement)
	for configurationIndex := range state.Configurations {
		if !configurationAppliesToTarget(state.Configurations[configurationIndex], target) {
			continue
		}
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
	set := artifactrequirements.Set{
		Version: artifactrequirements.CurrentVersion, ArtifactSchemaVersion: schemaVersion,
		Target: cloneTargetPredicate(target),
	}
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

func targetPredicates(state models.State) ([]*artifactrequirements.TargetPredicate, error) {
	distros := map[string]bool{"": true}
	architectures := map[string]bool{"": true}
	for _, configuration := range state.Configurations {
		for _, distro := range configuration.TargetDistros {
			distros[strings.ToLower(string(distro))] = true
		}
		for _, architecture := range configuration.TargetArch {
			architectures[strings.ToLower(string(architecture))] = true
		}
	}
	distroValues := sortedTargetValues(distros)
	architectureValues := sortedTargetValues(architectures)
	if len(distroValues)*len(architectureValues) > MaxTargetPredicates {
		return nil, fmt.Errorf("target predicate count exceeds %d", MaxTargetPredicates)
	}
	var predicates []*artifactrequirements.TargetPredicate
	for _, distro := range distroValues {
		for _, architecture := range architectureValues {
			predicate := newTargetPredicate(distro, architecture)
			if anyConfigurationApplies(state.Configurations, predicate) {
				predicates = append(predicates, predicate)
			}
		}
	}
	if len(predicates) == 0 {
		return nil, fmt.Errorf("no applicable target predicates")
	}
	return predicates, nil
}

func sortedTargetValues(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func newTargetPredicate(distro, architecture string) *artifactrequirements.TargetPredicate {
	if distro == "" && architecture == "" {
		return nil
	}
	target := &artifactrequirements.TargetPredicate{}
	if distro != "" {
		target.Distros = []string{distro}
	}
	if architecture != "" {
		target.Architectures = []string{architecture}
	}
	return target
}

func anyConfigurationApplies(configurations []models.Configuration, target *artifactrequirements.TargetPredicate) bool {
	for _, configuration := range configurations {
		if configurationAppliesToTarget(configuration, target) {
			return true
		}
	}
	return false
}

func configurationAppliesToTarget(configuration models.Configuration, target *artifactrequirements.TargetPredicate) bool {
	distro := singleTargetValue(target, true)
	architecture := singleTargetValue(target, false)
	return targetDimensionApplies(configuration.TargetDistros, distro) && targetDimensionApplies(configuration.TargetArch, architecture)
}

func singleTargetValue(target *artifactrequirements.TargetPredicate, distro bool) string {
	if target == nil {
		return ""
	}
	values := target.Architectures
	if distro {
		values = target.Distros
	}
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func targetDimensionApplies[T ~string](declared []T, selected string) bool {
	if len(declared) == 0 {
		return true
	}
	if selected == "" {
		return false
	}
	for _, value := range declared {
		if strings.EqualFold(string(value), selected) {
			return true
		}
	}
	return false
}

func cloneTargetPredicate(target *artifactrequirements.TargetPredicate) *artifactrequirements.TargetPredicate {
	if target == nil {
		return nil
	}
	return &artifactrequirements.TargetPredicate{
		Distros: append([]string(nil), target.Distros...), Architectures: append([]string(nil), target.Architectures...),
	}
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
