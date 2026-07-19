package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DavidHoenisch/remotr/internal/artifactrequirements"
	"github.com/DavidHoenisch/remotr/internal/artifactvariant"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

func BenchmarkCompiledArtifactVariantsDatabaseBoundedBySchema(b *testing.B) {
	databaseURL := os.Getenv("REMOTR_BENCH_DATABASE_URL")
	if databaseURL == "" {
		b.Skip("REMOTR_BENCH_DATABASE_URL is required for the Postgres benchmark")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = tx.Rollback(ctx) })
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE compiled_artifact_variants (
id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
fleet_name TEXT,
endpoint_id TEXT,
release_ref TEXT NOT NULL,
artifact_type TEXT NOT NULL,
schema_version INTEGER NOT NULL,
source_digest TEXT NOT NULL,
requirement_set_digest TEXT NOT NULL,
requirement_set JSONB NOT NULL,
artifact BYTEA NOT NULL,
digest TEXT NOT NULL,
compiled_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
) ON COMMIT DROP;
CREATE UNIQUE INDEX ON compiled_artifact_variants
(fleet_name, release_ref, artifact_type, schema_version, requirement_set_digest)
WHERE fleet_name IS NOT NULL;
CREATE UNIQUE INDEX ON compiled_artifact_variants
(endpoint_id, release_ref, artifact_type, schema_version, requirement_set_digest)
WHERE endpoint_id IS NOT NULL`); err != nil {
		b.Fatal(err)
	}
	store := NewFromArtifactVariantQueries(db.New(tx))
	variants := benchmarkSchemaVariants(b)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		for _, variant := range variants {
			if err := store.StoreCompiledArtifactVariantForFleet(ctx, "engineering", "release-benchmark", "desired", variant); err != nil {
				b.Fatal(err)
			}
		}
		stored, err := store.ListCompiledArtifactVariantsForFleet(ctx, "engineering", "release-benchmark", "desired")
		if err != nil || len(stored) != 2 {
			b.Fatalf("stored variants = %d, err=%v", len(stored), err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(len(variants)), "variant_rows")
	b.ReportMetric(400, "endpoint_population")
}

func benchmarkSchemaVariants(b *testing.B) []artifactvariant.Variant {
	b.Helper()
	var variants []artifactvariant.Variant
	for _, schemaVersion := range []int{1, 0} {
		requirements := artifactrequirements.Set{
			Version: artifactrequirements.CurrentVersion, ArtifactSchemaVersion: schemaVersion,
			ResourceCapabilities: []artifactrequirements.Requirement{{ID: "resource:package", Revision: "package-v1"}},
			ProviderCapabilities: []artifactrequirements.Requirement{{ID: "provider:package/apt", Revision: "1"}},
		}
		requirementDigest, err := requirements.CanonicalDigest()
		if err != nil {
			b.Fatal(err)
		}
		variants = append(variants, artifactvariant.Variant{
			Artifact: []byte("configurations: []\n"), Digest: "artifact-digest", SourceDigest: "source-digest",
			SchemaVersion: schemaVersion, Requirements: requirements, RequirementDigest: requirementDigest,
		})
	}
	return variants
}
