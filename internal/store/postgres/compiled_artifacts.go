package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

// ErrCompiledArtifactNotFound is returned when no cached artifact exists for the target.
var ErrCompiledArtifactNotFound = errors.New("compiled artifact not found")

// StoreCompiledArtifactForFleet upserts a fleet artifact at releaseRef.
func (s *Store) StoreCompiledArtifactForFleet(ctx context.Context, fleetName, releaseRef, artifactType string, artifact []byte, digest string) error {
	_, err := s.q.UpsertCompiledArtifactForFleet(ctx, db.UpsertCompiledArtifactForFleetParams{
		FleetName:    pgtype.Text{String: fleetName, Valid: fleetName != ""},
		ReleaseRef:   releaseRef,
		ArtifactType: artifactType,
		Artifact:     artifact,
		Digest:       digest,
	})
	return err
}

// StoreCompiledArtifactForEndpoint upserts an endpoint override artifact at releaseRef.
func (s *Store) StoreCompiledArtifactForEndpoint(ctx context.Context, endpointID, releaseRef, artifactType string, artifact []byte, digest string) error {
	_, err := s.q.UpsertCompiledArtifactForEndpoint(ctx, db.UpsertCompiledArtifactForEndpointParams{
		EndpointID:   pgtype.Text{String: endpointID, Valid: endpointID != ""},
		ReleaseRef:   releaseRef,
		ArtifactType: artifactType,
		Artifact:     artifact,
		Digest:       digest,
	})
	return err
}

// GetCompiledArtifactForFleet returns cached artifact bytes and digest.
func (s *Store) GetCompiledArtifactForFleet(ctx context.Context, fleet, releaseRef, artifactType string) ([]byte, string, error) {
	row, err := s.q.GetCompiledArtifactForFleet(ctx, db.GetCompiledArtifactForFleetParams{
		FleetName:    pgtype.Text{String: fleet, Valid: fleet != ""},
		ReleaseRef:   releaseRef,
		ArtifactType: artifactType,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrCompiledArtifactNotFound
		}
		return nil, "", err
	}
	return row.Artifact, row.Digest, nil
}

// GetCompiledArtifactForEndpoint returns cached artifact bytes and digest.
func (s *Store) GetCompiledArtifactForEndpoint(ctx context.Context, endpointID, releaseRef, artifactType string) ([]byte, string, error) {
	row, err := s.q.GetCompiledArtifactForEndpoint(ctx, db.GetCompiledArtifactForEndpointParams{
		EndpointID:   pgtype.Text{String: endpointID, Valid: endpointID != ""},
		ReleaseRef:   releaseRef,
		ArtifactType: artifactType,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrCompiledArtifactNotFound
		}
		return nil, "", err
	}
	return row.Artifact, row.Digest, nil
}

// PruneOldCompiledArtifacts deletes cached artifacts older than olderThan.
func (s *Store) PruneOldCompiledArtifacts(ctx context.Context, olderThan time.Time) error {
	return s.q.PruneOldCompiledArtifacts(ctx, pgtype.Timestamptz{Time: olderThan, Valid: true})
}

// StoreRenderedArtifacts upserts all rendered artifacts for one release ref.
func (s *Store) StoreRenderedArtifacts(ctx context.Context, releaseRef string, artifacts []RenderedArtifactPayload) error {
	for _, a := range artifacts {
		if a.FleetName != "" {
			if err := s.StoreCompiledArtifactForFleet(ctx, a.FleetName, releaseRef, a.ArtifactType, a.YAML, a.Digest); err != nil {
				return fmt.Errorf("fleet %s %s: %w", a.FleetName, a.ArtifactType, err)
			}
			continue
		}
		if a.EndpointID != "" {
			if err := s.StoreCompiledArtifactForEndpoint(ctx, a.EndpointID, releaseRef, a.ArtifactType, a.YAML, a.Digest); err != nil {
				return fmt.Errorf("endpoint %s %s: %w", a.EndpointID, a.ArtifactType, err)
			}
			continue
		}
		return fmt.Errorf("artifact payload missing fleet or endpoint id")
	}
	return nil
}

// RenderedArtifactPayload is one artifact to store in compiled_artifacts.
type RenderedArtifactPayload struct {
	FleetName    string
	EndpointID   string
	ArtifactType string
	YAML         []byte
	Digest       string
}
