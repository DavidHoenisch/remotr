package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/artifactrequirements"
	"github.com/DavidHoenisch/remotr/internal/artifactvariant"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

type fakeArtifactVariantQuerier struct {
	fleet db.UpsertCompiledArtifactVariantForFleetParams
}

func (f *fakeArtifactVariantQuerier) UpsertCompiledArtifactVariantForFleet(_ context.Context, params db.UpsertCompiledArtifactVariantForFleetParams) (db.CompiledArtifactVariant, error) {
	f.fleet = params
	return db.CompiledArtifactVariant{}, nil
}

func (f *fakeArtifactVariantQuerier) UpsertCompiledArtifactVariantForEndpoint(context.Context, db.UpsertCompiledArtifactVariantForEndpointParams) (db.CompiledArtifactVariant, error) {
	return db.CompiledArtifactVariant{}, nil
}

func (f *fakeArtifactVariantQuerier) ListCompiledArtifactVariantsForFleet(context.Context, db.ListCompiledArtifactVariantsForFleetParams) ([]db.CompiledArtifactVariant, error) {
	return nil, nil
}

func (f *fakeArtifactVariantQuerier) ListCompiledArtifactVariantsForEndpoint(context.Context, db.ListCompiledArtifactVariantsForEndpointParams) ([]db.CompiledArtifactVariant, error) {
	return nil, nil
}

func (f *fakeArtifactVariantQuerier) PruneOldCompiledArtifactVariants(context.Context, pgtype.Timestamptz) error {
	return nil
}

func TestCompiledArtifactVariantPersistsRequirementEvidence(t *testing.T) {
	requirements := artifactrequirements.Set{
		Version: artifactrequirements.CurrentVersion, ArtifactSchemaVersion: 1,
		ResourceCapabilities: []artifactrequirements.Requirement{{ID: "resource:package", Revision: "package-v1"}},
		ProviderCapabilities: []artifactrequirements.Requirement{{ID: "provider:package/apt", Revision: "1"}},
	}
	requirementDigest, err := requirements.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	queries := &fakeArtifactVariantQuerier{}
	store := NewFromArtifactVariantQueries(queries)
	err = store.StoreCompiledArtifactVariantForFleet(t.Context(), "engineering", "release-1", "desired", artifactvariant.Variant{
		Artifact: []byte("schemaVersion: 1\nconfigurations: []\n"), Digest: "artifact-digest", SourceDigest: "source-digest",
		SchemaVersion: 1, Requirements: requirements, RequirementDigest: requirementDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if queries.fleet.FleetName.String != "engineering" || queries.fleet.ReleaseRef != "release-1" ||
		queries.fleet.SchemaVersion != 1 || queries.fleet.SourceDigest != "source-digest" ||
		queries.fleet.RequirementSetDigest != requirementDigest || len(queries.fleet.RequirementSet) == 0 {
		t.Fatalf("persisted variant params = %+v", queries.fleet)
	}
}
