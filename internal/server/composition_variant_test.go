package server

import (
	"context"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/artifactvariant"
)

type storedVariant struct {
	Fleet        string
	ReleaseRef   string
	ArtifactType string
	Variant      artifactvariant.Variant
}

type capturingVariantStore struct {
	missArtifactStore
	variants []storedVariant
}

func (s *capturingVariantStore) StoreCompiledArtifactVariantForFleet(_ context.Context, fleet, releaseRef, artifactType string, variant artifactvariant.Variant) error {
	s.variants = append(s.variants, storedVariant{Fleet: fleet, ReleaseRef: releaseRef, ArtifactType: artifactType, Variant: variant})
	return nil
}

func (s *capturingVariantStore) StoreCompiledArtifactVariantForEndpoint(context.Context, string, string, string, artifactvariant.Variant) error {
	return nil
}

func TestCompositionPersistsBoundedSchemaVariants(t *testing.T) {
	repo := t.TempDir()
	writeTestFleetDesired(t, repo, "engineering", `configurations:
  - name: base
    packages:
      - name: curl
        present: true
        packageManager: apt
`)
	store := &capturingVariantStore{}
	service := &CompositionService{RepoRoot: repo, Store: store}
	if err := service.ComposeAll(t.Context(), "release-variant"); err != nil {
		t.Fatal(err)
	}
	if len(store.variants) != 2 {
		t.Fatalf("persisted variant count = %d, want schema 1 and schema 0", len(store.variants))
	}
	for index, schemaVersion := range []int{1, 0} {
		stored := store.variants[index]
		if stored.Fleet != "engineering" || stored.ReleaseRef != "release-variant" || stored.ArtifactType != "desired" ||
			stored.Variant.SchemaVersion != schemaVersion || stored.Variant.RequirementDigest == "" {
			t.Fatalf("persisted variant %d = %+v", index, stored)
		}
	}
}
