package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/configcompose"
	pgstore "github.com/DavidHoenisch/remotr/internal/store/postgres"
)

// ArtifactStore loads and persists compiled deployable artifacts.
type ArtifactStore interface {
	StoreCompiledArtifactForFleet(ctx context.Context, fleetName, releaseRef, artifactType string, artifact []byte, digest string) error
	StoreCompiledArtifactForEndpoint(ctx context.Context, endpointID, releaseRef, artifactType string, artifact []byte, digest string) error
	GetCompiledArtifactForFleet(ctx context.Context, fleet, releaseRef, artifactType string) ([]byte, string, error)
	GetCompiledArtifactForEndpoint(ctx context.Context, endpointID, releaseRef, artifactType string) ([]byte, string, error)
	PruneOldCompiledArtifacts(ctx context.Context, olderThan time.Time) error
}

// CompositionService composes and caches deployable artifacts when release ref advances.
type CompositionService struct {
	RepoRoot string
	Store    ArtifactStore
}

// ComposeAll composes every fleet and endpoint override and stores results for releaseRef.
func (c *CompositionService) ComposeAll(ctx context.Context, releaseRef string) error {
	if c == nil || c.Store == nil {
		return nil
	}
	repoRoot := c.RepoRoot
	artifacts, err := configcompose.RenderAll(repoRoot)
	if err != nil {
		return err
	}
	for _, a := range artifacts {
		switch a.TargetType {
		case "fleet":
			if err := c.Store.StoreCompiledArtifactForFleet(ctx, a.TargetID, releaseRef, a.ArtifactType, a.YAML, a.Digest); err != nil {
				return fmt.Errorf("fleet %s %s: %w", a.TargetID, a.ArtifactType, err)
			}
		case "endpoint":
			if err := c.Store.StoreCompiledArtifactForEndpoint(ctx, a.TargetID, releaseRef, a.ArtifactType, a.YAML, a.Digest); err != nil {
				return fmt.Errorf("endpoint %s %s: %w", a.TargetID, a.ArtifactType, err)
			}
		default:
			return fmt.Errorf("unknown target type %q", a.TargetType)
		}
	}
	pruneAge := envArtifactPruneAge()
	if pruneAge > 0 {
		cutoff := time.Now().UTC().Add(-pruneAge)
		if err := c.Store.PruneOldCompiledArtifacts(ctx, cutoff); err != nil {
			slog.Warn("prune compiled artifacts", "err", err)
		}
	}
	return nil
}

func envArtifactPruneAge() time.Duration {
	v := os.Getenv("REMOTR_ARTIFACT_PRUNE_AGE")
	if v == "" {
		return 90 * 24 * time.Hour
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		slog.Warn("invalid REMOTR_ARTIFACT_PRUNE_AGE", "value", v, "err", err)
		return 90 * 24 * time.Hour
	}
	return d
}

// OnDemandArtifactResolver renders artifacts from the repo when no cache is available.
type OnDemandArtifactResolver struct {
	RepoRoot string
}

func (r *OnDemandArtifactResolver) GetCompiledArtifactForFleet(_ context.Context, fleet, _releaseRef, artifactType string) ([]byte, string, error) {
	desired, crons, desiredDigest, cronsDigest, err := configcompose.RenderFleet(r.RepoRoot, fleet)
	if err != nil {
		return nil, "", err
	}
	switch artifactType {
	case "desired":
		return desired, desiredDigest, nil
	case "crons":
		if len(crons) == 0 {
			return nil, "", pgstore.ErrCompiledArtifactNotFound
		}
		return crons, cronsDigest, nil
	default:
		return nil, "", fmt.Errorf("unknown artifact type %q", artifactType)
	}
}

func (r *OnDemandArtifactResolver) GetCompiledArtifactForEndpoint(_ context.Context, endpointID, _releaseRef, artifactType string) ([]byte, string, error) {
	manifestDir := filepath.Join("endpoints", endpointID)
	dirPath := filepath.Join(r.RepoRoot, filepath.FromSlash(manifestDir))
	if _, err := os.Stat(dirPath); err != nil {
		if os.IsNotExist(err) {
			return nil, "", pgstore.ErrCompiledArtifactNotFound
		}
		return nil, "", err
	}
	_, err := configcompose.FindManifestInTree(r.RepoRoot, manifestDir)
	if err != nil {
		if errors.Is(err, configcompose.ErrNoManifest) {
			return nil, "", pgstore.ErrCompiledArtifactNotFound
		}
		return nil, "", err
	}
	desired, crons, desiredDigest, cronsDigest, err := configcompose.RenderEndpoint(r.RepoRoot, endpointID)
	if err != nil {
		return nil, "", err
	}
	switch artifactType {
	case "desired":
		return desired, desiredDigest, nil
	case "crons":
		if len(crons) == 0 {
			return nil, "", pgstore.ErrCompiledArtifactNotFound
		}
		return crons, cronsDigest, nil
	default:
		return nil, "", fmt.Errorf("unknown artifact type %q", artifactType)
	}
}

func (r *OnDemandArtifactResolver) StoreCompiledArtifactForFleet(context.Context, string, string, string, []byte, string) error {
	return nil
}

func (r *OnDemandArtifactResolver) StoreCompiledArtifactForEndpoint(context.Context, string, string, string, []byte, string) error {
	return nil
}

func (r *OnDemandArtifactResolver) PruneOldCompiledArtifacts(context.Context, time.Time) error {
	return nil
}

// resolveDesiredArtifact returns endpoint override or fleet desired artifact.
func resolveDesiredArtifact(ctx context.Context, store ArtifactStore, repoRoot, fleet, endpointID, releaseRef string) ([]byte, string, error) {
	if store == nil {
		return nil, "", fmt.Errorf("artifact store not configured")
	}
	artifact, digest, err := getDesiredFromStore(ctx, store, fleet, endpointID, releaseRef)
	if err == nil {
		return artifact, digest, nil
	}
	if !errors.Is(err, pgstore.ErrCompiledArtifactNotFound) || strings.TrimSpace(repoRoot) == "" {
		return nil, "", err
	}
	onDemand := &OnDemandArtifactResolver{RepoRoot: repoRoot}
	return getDesiredFromStore(ctx, onDemand, fleet, endpointID, releaseRef)
}

func getDesiredFromStore(ctx context.Context, store ArtifactStore, fleet, endpointID, releaseRef string) ([]byte, string, error) {
	artifact, digest, err := store.GetCompiledArtifactForEndpoint(ctx, endpointID, releaseRef, "desired")
	if err == nil {
		return artifact, digest, nil
	}
	if !errors.Is(err, pgstore.ErrCompiledArtifactNotFound) {
		return nil, "", err
	}
	return store.GetCompiledArtifactForFleet(ctx, fleet, releaseRef, "desired")
}

// resolveCronsArtifact returns endpoint override or fleet crons artifact.
func resolveCronsArtifact(ctx context.Context, store ArtifactStore, repoRoot, fleet, endpointID, releaseRef string) ([]byte, string, bool, error) {
	if store == nil {
		return nil, "", false, fmt.Errorf("artifact store not configured")
	}
	artifact, digest, ok, err := getCronsFromStore(ctx, store, fleet, endpointID, releaseRef)
	if err == nil {
		return artifact, digest, ok, nil
	}
	if !errors.Is(err, pgstore.ErrCompiledArtifactNotFound) || strings.TrimSpace(repoRoot) == "" {
		return nil, "", false, err
	}
	onDemand := &OnDemandArtifactResolver{RepoRoot: repoRoot}
	artifact, digest, ok, err = getCronsFromStore(ctx, onDemand, fleet, endpointID, releaseRef)
	if errors.Is(err, pgstore.ErrCompiledArtifactNotFound) {
		return nil, "", false, nil
	}
	return artifact, digest, ok, err
}

func getCronsFromStore(ctx context.Context, store ArtifactStore, fleet, endpointID, releaseRef string) ([]byte, string, bool, error) {
	artifact, digest, err := store.GetCompiledArtifactForEndpoint(ctx, endpointID, releaseRef, "crons")
	if err == nil {
		return artifact, digest, true, nil
	}
	if !errors.Is(err, pgstore.ErrCompiledArtifactNotFound) {
		return nil, "", false, err
	}
	artifact, digest, err = store.GetCompiledArtifactForFleet(ctx, fleet, releaseRef, "crons")
	if err == nil {
		return artifact, digest, true, nil
	}
	if errors.Is(err, pgstore.ErrCompiledArtifactNotFound) {
		return nil, "", false, pgstore.ErrCompiledArtifactNotFound
	}
	return nil, "", false, err
}
