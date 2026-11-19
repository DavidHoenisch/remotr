package configcompose

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
	"gopkg.in/yaml.v3"
)

func loadQueriesFile(repoRoot, relPath string) (models.QueryState, error) {
	data, err := readRepoRelative(repoRoot, relPath)
	if err != nil {
		return models.QueryState{}, err
	}
	state, err := models.ParseQueryState(bytes.NewReader(data))
	if err != nil {
		return models.QueryState{}, fmt.Errorf("parse queries file %q: %w", relPath, err)
	}
	if state.Kind != types.KindQueries {
		return models.QueryState{}, fmt.Errorf("parse queries file %q: want kind %s, got %q", relPath, types.KindQueries, state.Kind)
	}
	return state, nil
}

func composeQueriesFromManifest(repoRoot, manifestRel string) (models.QueryState, error) {
	merged, err := resolveManifestChain(repoRoot, manifestRel, map[string]struct{}{})
	if err != nil {
		return models.QueryState{}, err
	}
	if len(merged.Queries) == 0 {
		return models.QueryState{}, nil
	}

	manifestDir := filepath.Dir(manifestRel)
	queryPaths, err := resolveQueryRefs(repoRoot, manifestDir, merged.Queries)
	if err != nil {
		return models.QueryState{}, fmt.Errorf("manifest %q: %w", manifestRel, err)
	}

	var queries []models.QuerySpec
	seen := map[string]struct{}{}
	for _, queryPath := range queryPaths {
		file, err := loadQueriesFile(repoRoot, queryPath)
		if err != nil {
			return models.QueryState{}, err
		}
		for _, q := range file.Queries {
			if _, dup := seen[q.Name]; dup {
				return models.QueryState{}, fmt.Errorf("duplicate query %q (file %q)", q.Name, queryPath)
			}
			seen[q.Name] = struct{}{}
			queries = append(queries, q)
		}
	}
	if len(queries) == 0 {
		return models.QueryState{}, fmt.Errorf("manifest %q: composed queries are empty", manifestRel)
	}
	state := models.QueryState{
		Kind:          types.KindQueries,
		SchemaVersion: 1,
		Queries:       queries,
	}
	if err := state.Validate(); err != nil {
		return models.QueryState{}, fmt.Errorf("validate queries: %w", err)
	}
	return state, nil
}

func marshalQueryState(state models.QueryState) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(state); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RenderManifestQueries composes the optional queries artifact for one manifest path.
func RenderManifestQueries(repoRoot, manifestRel string) ([]byte, string, error) {
	manifestRel = normalizeRelPath(manifestRel)
	state, err := composeQueriesFromManifest(repoRoot, manifestRel)
	if err != nil {
		return nil, "", err
	}
	if len(state.Queries) == 0 {
		return nil, "", nil
	}
	raw, err := marshalQueryState(state)
	if err != nil {
		return nil, "", err
	}
	if _, err := models.ParseQueryState(bytes.NewReader(raw)); err != nil {
		return nil, "", fmt.Errorf("validate rendered queries: %w", err)
	}
	return raw, digestBytes(raw), nil
}

// RenderFleetQueries composes the queries artifact for fleets/<fleetName>/.
func RenderFleetQueries(repoRoot, fleetName string) ([]byte, string, error) {
	if err := configrepo.ValidateFleetName(fleetName); err != nil {
		return nil, "", err
	}
	manifest, err := FindManifestInTree(repoRoot, filepath.Join("fleets", fleetName))
	if err != nil {
		return nil, "", fmt.Errorf("fleet %q: %w", fleetName, err)
	}
	return RenderManifestQueries(repoRoot, manifest)
}

// RenderEndpointQueries composes the queries artifact for endpoints/<endpointID>/.
func RenderEndpointQueries(repoRoot, endpointID string) ([]byte, string, error) {
	if err := configrepo.ValidateEndpointID(endpointID); err != nil {
		return nil, "", err
	}
	manifest, err := FindManifestInTree(repoRoot, filepath.Join("endpoints", endpointID))
	if err != nil {
		return nil, "", fmt.Errorf("endpoint %q: %w", endpointID, err)
	}
	return RenderManifestQueries(repoRoot, manifest)
}
