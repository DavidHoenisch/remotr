package configcompose

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/capabilitymatrix"
	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
)

// RenderedArtifact is one composed deployable output.
type RenderedArtifact struct {
	TargetType   string // fleet or endpoint
	TargetID     string // fleet name or endpoint id
	ArtifactType string // desired or crons
	YAML         []byte
	Digest       string
}

// CompositionTarget identifies one fleet or endpoint override to compose.
type CompositionTarget struct {
	FleetName  string
	EndpointID string
	Manifest   string
}

// RenderManifest composes desired and optional crons artifacts for one manifest path.
func RenderManifest(repoRoot, manifestRel string) (desired, crons []byte, desiredDigest, cronsDigest string, err error) {
	manifestRel = normalizeRelPath(manifestRel)
	state, err := composeManifest(repoRoot, manifestRel)
	if err != nil {
		return nil, nil, "", "", err
	}
	if err := configrepo.ValidateState(state, manifestRel); err != nil {
		return nil, nil, "", "", fmt.Errorf("validate desired: %w", err)
	}
	desired, err = marshalState(state)
	if err != nil {
		return nil, nil, "", "", err
	}
	if _, err := models.ParseState(bytes.NewReader(desired)); err != nil {
		return nil, nil, "", "", fmt.Errorf("validate rendered desired: %w", err)
	}
	desiredDigest = digestBytes(desired)

	cronState, err := composeCronsFromManifest(repoRoot, manifestRel)
	if err != nil {
		return nil, nil, "", "", err
	}
	if len(cronState.Crons) == 0 {
		return desired, nil, desiredDigest, "", nil
	}
	if err := configrepo.ValidateComposedCronState(cronState, manifestRel); err != nil {
		return nil, nil, "", "", fmt.Errorf("validate crons: %w", err)
	}
	crons, err = marshalCronState(cronState)
	if err != nil {
		return nil, nil, "", "", err
	}
	cronsDigest = digestBytes(crons)
	return desired, crons, desiredDigest, cronsDigest, nil
}

// RenderFleet composes artifacts for fleets/<fleetName>/.
func RenderFleet(repoRoot, fleetName string) (desired, crons []byte, desiredDigest, cronsDigest string, err error) {
	if err := configrepo.ValidateFleetName(fleetName); err != nil {
		return nil, nil, "", "", err
	}
	manifest, err := FindManifestInTree(repoRoot, filepath.Join("fleets", fleetName))
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("fleet %q: %w", fleetName, err)
	}
	return RenderManifest(repoRoot, manifest)
}

// RenderEndpoint composes artifacts for endpoints/<endpointID>/.
func RenderEndpoint(repoRoot, endpointID string) (desired, crons []byte, desiredDigest, cronsDigest string, err error) {
	if err := configrepo.ValidateEndpointID(endpointID); err != nil {
		return nil, nil, "", "", err
	}
	manifest, err := FindManifestInTree(repoRoot, filepath.Join("endpoints", endpointID))
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("endpoint %q: %w", endpointID, err)
	}
	return RenderManifest(repoRoot, manifest)
}

// ListCompositionTargets returns all fleet and endpoint override manifests in the repo.
func ListCompositionTargets(repoRoot string) ([]CompositionTarget, error) {
	repoRoot, err := absRepoRoot(repoRoot)
	if err != nil {
		return nil, err
	}
	var out []CompositionTarget
	fleets, err := ListFleetNames(repoRoot)
	if err != nil {
		return nil, err
	}
	for _, fleet := range fleets {
		manifest, err := FindManifestInTree(repoRoot, filepath.Join("fleets", fleet))
		if err != nil {
			return nil, fmt.Errorf("fleet %q: %w", fleet, err)
		}
		out = append(out, CompositionTarget{FleetName: fleet, Manifest: manifest})
	}
	endpoints, err := ListEndpointOverrideIDs(repoRoot)
	if err != nil {
		return nil, err
	}
	for _, id := range endpoints {
		manifest, err := FindManifestInTree(repoRoot, filepath.Join("endpoints", id))
		if err != nil {
			return nil, fmt.Errorf("endpoint %q: %w", id, err)
		}
		out = append(out, CompositionTarget{EndpointID: id, Manifest: manifest})
	}
	return out, nil
}

// RenderAll composes every fleet and endpoint override in the repository.
func RenderAll(repoRoot string) ([]RenderedArtifact, error) {
	targets, err := ListCompositionTargets(repoRoot)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no fleet manifests found")
	}
	var out []RenderedArtifact
	for _, target := range targets {
		artifacts, err := renderTarget(repoRoot, target)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", target.Manifest, err)
		}
		out = append(out, artifacts...)
	}
	return out, nil
}

func renderTarget(repoRoot string, target CompositionTarget) ([]RenderedArtifact, error) {
	desired, crons, desiredDigest, cronsDigest, err := RenderManifest(repoRoot, target.Manifest)
	if err != nil {
		return nil, err
	}
	targetType := "fleet"
	targetID := target.FleetName
	if target.EndpointID != "" {
		targetType = "endpoint"
		targetID = target.EndpointID
	}
	var out []RenderedArtifact
	out = append(out, RenderedArtifact{
		TargetType:   targetType,
		TargetID:     targetID,
		ArtifactType: "desired",
		YAML:         desired,
		Digest:       desiredDigest,
	})
	if len(crons) > 0 {
		out = append(out, RenderedArtifact{
			TargetType:   targetType,
			TargetID:     targetID,
			ArtifactType: "crons",
			YAML:         crons,
			Digest:       cronsDigest,
		})
	}
	return out, nil
}

// HasManifests reports whether repoRoot contains any kind: manifest sources.
func HasManifests(repoRoot string) (bool, error) {
	repoRoot, err := absRepoRoot(repoRoot)
	if err != nil {
		return false, err
	}
	fleets, err := ListFleetNames(repoRoot)
	if err != nil {
		return false, err
	}
	for _, fleet := range fleets {
		if _, err := FindManifestInTree(repoRoot, filepath.Join("fleets", fleet)); err == nil {
			return true, nil
		}
	}
	endpoints, err := ListEndpointOverrideIDs(repoRoot)
	if err != nil {
		return false, err
	}
	return len(endpoints) > 0, nil
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// FleetDiscoverSummary lists discovered files under a fleet directory.
type FleetDiscoverSummary struct {
	Fleet                  string                `json:"fleet"`
	Manifest               string                `json:"manifest"`
	Modules                []string              `json:"modules,omitempty"`
	Applications           []string              `json:"applications,omitempty"`
	Crons                  []string              `json:"crons,omitempty"`
	ResourceKinds          []models.ResourceKind `json:"resourceKinds,omitempty"`
	CapabilityRequirements []string              `json:"capabilityRequirements,omitempty"`
	Diagnostics            []models.Diagnostic   `json:"diagnostics,omitempty"`
}

// DiscoverFleet returns kind-grouped files referenced from a fleet manifest tree.
func DiscoverFleet(repoRoot, fleetName string) (FleetDiscoverSummary, error) {
	if err := configrepo.ValidateFleetName(fleetName); err != nil {
		return FleetDiscoverSummary{}, err
	}
	fleetDir := filepath.Join("fleets", fleetName)
	manifest, err := FindManifestInTree(repoRoot, fleetDir)
	if err != nil {
		return FleetDiscoverSummary{}, err
	}
	merged, err := resolveManifestChain(repoRoot, manifest, map[string]struct{}{})
	if err != nil {
		return FleetDiscoverSummary{}, err
	}
	manifestDir := filepath.Dir(manifest)
	modules, err := resolveModuleRefs(repoRoot, manifestDir, merged.Modules)
	if err != nil {
		return FleetDiscoverSummary{}, err
	}
	apps, err := resolveApplicationRefs(repoRoot, manifestDir, merged.Applications)
	if err != nil {
		return FleetDiscoverSummary{}, err
	}
	crons, err := resolveCronRefs(repoRoot, manifestDir, merged.Crons)
	if err != nil {
		return FleetDiscoverSummary{}, err
	}
	state, err := composeManifest(repoRoot, manifest)
	if err != nil {
		return FleetDiscoverSummary{}, err
	}
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		return FleetDiscoverSummary{}, err
	}
	kindSet := map[models.ResourceKind]struct{}{}
	requirementSet := map[string]struct{}{}
	for i := range state.Configurations {
		resources, err := registry.Resources(&state.Configurations[i])
		if err != nil {
			return FleetDiscoverSummary{}, err
		}
		for _, resource := range resources {
			kindSet[resource.Kind()] = struct{}{}
			for _, requirement := range capabilitymatrix.Requirements(resource.Kind(), resource.Value()) {
				requirementSet[requirement] = struct{}{}
			}
		}
	}
	kinds := make([]models.ResourceKind, 0, len(kindSet))
	for kind := range kindSet {
		kinds = append(kinds, kind)
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })
	requirements := make([]string, 0, len(requirementSet))
	for requirement := range requirementSet {
		requirements = append(requirements, requirement)
	}
	sort.Strings(requirements)
	return FleetDiscoverSummary{
		Fleet:                  fleetName,
		Manifest:               manifest,
		Modules:                modules,
		Applications:           apps,
		Crons:                  crons,
		ResourceKinds:          kinds,
		CapabilityRequirements: requirements,
		Diagnostics:            append([]models.Diagnostic(nil), state.Diagnostics...),
	}, nil
}

// ValidateComposition runs render for all targets and returns issues without writing.
func ValidateComposition(repoRoot string) (Result, error) {
	repoRoot, err := absRepoRoot(repoRoot)
	if err != nil {
		return Result{}, err
	}
	targets, err := ListCompositionTargets(repoRoot)
	if err != nil {
		return Result{RepoRoot: repoRoot, Issues: []Issue{{Path: repoRoot, Message: err.Error()}}}, nil
	}
	if len(targets) == 0 {
		return Result{
			RepoRoot: repoRoot,
			Issues:   []Issue{{Path: repoRoot, Message: "no fleet manifests found"}},
		}, nil
	}
	res := Result{RepoRoot: repoRoot}
	for _, target := range targets {
		if _, err := renderTarget(repoRoot, target); err != nil {
			res.Issues = append(res.Issues, Issue{Path: target.Manifest, Message: err.Error()})
		} else {
			res.OK = append(res.OK, target.Manifest)
		}
	}
	return res, nil
}

// RenderStdout renders artifacts for CLI preview (optional fleet filter).
func RenderStdout(repoRoot, fleet, stdoutMode string) (Result, error) {
	repoRoot, err := absRepoRoot(repoRoot)
	if err != nil {
		return Result{}, err
	}
	stdoutMode = strings.TrimSpace(strings.ToLower(stdoutMode))
	if stdoutMode == "" {
		stdoutMode = "desired"
	}
	targets, err := ListCompositionTargets(repoRoot)
	if err != nil {
		return Result{}, err
	}
	fleet = strings.TrimSpace(fleet)
	if fleet != "" {
		if err := configrepo.ValidateFleetName(fleet); err != nil {
			return Result{}, fmt.Errorf("fleet: %w", err)
		}
		filtered := make([]CompositionTarget, 0, len(targets))
		for _, t := range targets {
			if t.FleetName == fleet {
				filtered = append(filtered, t)
				continue
			}
			if t.EndpointID != "" {
				ok, err := manifestExtendsFleet(repoRoot, t.Manifest, fleet)
				if err != nil {
					return Result{}, err
				}
				if ok {
					filtered = append(filtered, t)
				}
			}
		}
		targets = filtered
	}
	res := Result{RepoRoot: repoRoot}
	wantDesired := stdoutMode == "desired" || stdoutMode == "all"
	wantCrons := stdoutMode == "crons" || stdoutMode == "all"
	for _, target := range targets {
		artifacts, err := renderTarget(repoRoot, target)
		if err != nil {
			res.Issues = append(res.Issues, Issue{Path: target.Manifest, Message: err.Error()})
			continue
		}
		for _, a := range artifacts {
			if a.ArtifactType == "desired" && !wantDesired {
				continue
			}
			if a.ArtifactType == "crons" && !wantCrons {
				continue
			}
			label := target.Manifest
			if a.ArtifactType == "crons" {
				label = label + " (crons)"
			}
			res.Rendered = append(res.Rendered, Rendered{
				Path:    label,
				Content: string(a.YAML),
			})
		}
	}
	return res, nil
}
