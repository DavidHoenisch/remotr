package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/artifactrequirements"
	"github.com/DavidHoenisch/remotr/internal/artifactvariant"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

// ArtifactVariantQuerier is the generated persistence surface for bounded
// schema variants.
type ArtifactVariantQuerier interface {
	UpsertCompiledArtifactVariantForFleet(context.Context, db.UpsertCompiledArtifactVariantForFleetParams) (db.CompiledArtifactVariant, error)
	UpsertCompiledArtifactVariantForEndpoint(context.Context, db.UpsertCompiledArtifactVariantForEndpointParams) (db.CompiledArtifactVariant, error)
	ListCompiledArtifactVariantsForFleet(context.Context, db.ListCompiledArtifactVariantsForFleetParams) ([]db.CompiledArtifactVariant, error)
	ListCompiledArtifactVariantsForEndpoint(context.Context, db.ListCompiledArtifactVariantsForEndpointParams) ([]db.CompiledArtifactVariant, error)
	PruneOldCompiledArtifactVariants(context.Context, pgtype.Timestamptz) error
}

func (s *Store) StoreCompiledArtifactVariantForFleet(ctx context.Context, fleetName, releaseRef, artifactType string, variant artifactvariant.Variant) error {
	body, err := validatedRequirementEvidence(variant)
	if err != nil {
		return err
	}
	if s.artifactVariantQ == nil {
		return fmt.Errorf("artifact variant persistence is not configured")
	}
	_, err = s.artifactVariantQ.UpsertCompiledArtifactVariantForFleet(ctx, db.UpsertCompiledArtifactVariantForFleetParams{
		FleetName: pgtype.Text{String: fleetName, Valid: fleetName != ""}, ReleaseRef: releaseRef, ArtifactType: artifactType,
		SchemaVersion: int32(variant.SchemaVersion), SourceDigest: variant.SourceDigest,
		RequirementSetDigest: variant.RequirementDigest, RequirementSet: body,
		Artifact: variant.Artifact, Digest: variant.Digest,
	})
	return err
}

func (s *Store) StoreCompiledArtifactVariantForEndpoint(ctx context.Context, endpointID, releaseRef, artifactType string, variant artifactvariant.Variant) error {
	body, err := validatedRequirementEvidence(variant)
	if err != nil {
		return err
	}
	if s.artifactVariantQ == nil {
		return fmt.Errorf("artifact variant persistence is not configured")
	}
	_, err = s.artifactVariantQ.UpsertCompiledArtifactVariantForEndpoint(ctx, db.UpsertCompiledArtifactVariantForEndpointParams{
		EndpointID: pgtype.Text{String: endpointID, Valid: endpointID != ""}, ReleaseRef: releaseRef, ArtifactType: artifactType,
		SchemaVersion: int32(variant.SchemaVersion), SourceDigest: variant.SourceDigest,
		RequirementSetDigest: variant.RequirementDigest, RequirementSet: body,
		Artifact: variant.Artifact, Digest: variant.Digest,
	})
	return err
}

func (s *Store) ListCompiledArtifactVariantsForFleet(ctx context.Context, fleetName, releaseRef, artifactType string) ([]artifactvariant.Variant, error) {
	if s.artifactVariantQ == nil {
		return nil, fmt.Errorf("artifact variant persistence is not configured")
	}
	rows, err := s.artifactVariantQ.ListCompiledArtifactVariantsForFleet(ctx, db.ListCompiledArtifactVariantsForFleetParams{
		FleetName: pgtype.Text{String: fleetName, Valid: fleetName != ""}, ReleaseRef: releaseRef, ArtifactType: artifactType,
	})
	if err != nil {
		return nil, err
	}
	return artifactVariantsFromRows(rows)
}

func (s *Store) ListCompiledArtifactVariantsForEndpoint(ctx context.Context, endpointID, releaseRef, artifactType string) ([]artifactvariant.Variant, error) {
	if s.artifactVariantQ == nil {
		return nil, fmt.Errorf("artifact variant persistence is not configured")
	}
	rows, err := s.artifactVariantQ.ListCompiledArtifactVariantsForEndpoint(ctx, db.ListCompiledArtifactVariantsForEndpointParams{
		EndpointID: pgtype.Text{String: endpointID, Valid: endpointID != ""}, ReleaseRef: releaseRef, ArtifactType: artifactType,
	})
	if err != nil {
		return nil, err
	}
	return artifactVariantsFromRows(rows)
}

func (s *Store) pruneOldCompiledArtifactVariants(ctx context.Context, olderThan time.Time) error {
	if s.artifactVariantQ == nil {
		return nil
	}
	return s.artifactVariantQ.PruneOldCompiledArtifactVariants(ctx, pgtype.Timestamptz{Time: olderThan, Valid: true})
}

func validatedRequirementEvidence(variant artifactvariant.Variant) ([]byte, error) {
	if variant.SchemaVersion != variant.Requirements.ArtifactSchemaVersion {
		return nil, fmt.Errorf("artifact schema does not match requirement set")
	}
	body, err := variant.Requirements.CanonicalBody()
	if err != nil {
		return nil, err
	}
	digest, err := variant.Requirements.CanonicalDigest()
	if err != nil {
		return nil, err
	}
	if digest != variant.RequirementDigest {
		return nil, fmt.Errorf("artifact requirement-set digest mismatch")
	}
	return body, nil
}

func artifactVariantsFromRows(rows []db.CompiledArtifactVariant) ([]artifactvariant.Variant, error) {
	variants := make([]artifactvariant.Variant, 0, len(rows))
	for _, row := range rows {
		requirements, err := artifactrequirements.DecodeCanonical(row.RequirementSet, row.RequirementSetDigest)
		if err != nil {
			return nil, fmt.Errorf("invalid stored artifact requirement set: %w", err)
		}
		if int(row.SchemaVersion) != requirements.ArtifactSchemaVersion {
			return nil, fmt.Errorf("stored artifact schema does not match requirement set")
		}
		variants = append(variants, artifactvariant.Variant{
			Artifact: append([]byte(nil), row.Artifact...), Digest: row.Digest, SourceDigest: row.SourceDigest,
			SchemaVersion: int(row.SchemaVersion), Requirements: requirements, RequirementDigest: row.RequirementSetDigest,
		})
	}
	return variants, nil
}
