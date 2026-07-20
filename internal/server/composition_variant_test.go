package server

import (
	"context"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/artifactvariant"
	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
	"github.com/DavidHoenisch/remotr/internal/providermatrix"
)

func TestCompositionRejectsUnqualifiedProviderBeforeArtifactStorage(t *testing.T) {
	repo := t.TempDir()
	writeTestFleetDesired(t, repo, "engineering", `configurations:
  - name: base
    targetDistros: [Debian]
    targetArch: [x86]
    packages:
      - name: curl
        present: true
        packageManager: apt
`)
	store := &capturingVariantStore{}
	emptyMatrix := providermatrix.Matrix{Version: 1}
	err := (&CompositionService{RepoRoot: repo, Store: store, ProviderMatrix: &emptyMatrix}).ComposeAll(t.Context(), "release-unqualified")
	if err == nil || !strings.Contains(err.Error(), "missing passing provider evidence") {
		t.Fatalf("ComposeAll() error = %v, want provider release rejection", err)
	}
	if len(store.variants) != 0 {
		t.Fatalf("stored %d variants before provider release validation", len(store.variants))
	}
}

type storedVariant struct {
	Fleet        string
	ReleaseRef   string
	ArtifactType string
	Variant      artifactvariant.Variant
}

func TestCompiledArtifactVariantsRemainSchemaBounded(t *testing.T) {
	repo := t.TempDir()
	writeTestFleetDesired(t, repo, "engineering", `configurations:
  - name: base
    targetDistros: [Debian]
    targetArch: [x86]
    packages:
      - name: curl
        present: true
        packageManager: apt
`)
	store := &capturingVariantStore{}
	if err := (&CompositionService{RepoRoot: repo, Store: store, ProviderMatrix: qualifiedAPTTestMatrix()}).ComposeAll(t.Context(), "release-bounded"); err != nil {
		t.Fatal(err)
	}
	if len(store.variants) != 2 {
		t.Fatalf("compiled variants = %d, want declared schema bound 2", len(store.variants))
	}
	variants := []artifactvariant.Variant{store.variants[0].Variant, store.variants[1].Variant}
	document, err := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{0, 1}, AgentVersion: "v1.2.3",
		Capabilities: []capabilitydoc.Capability{
			{ID: "resource:package", Revision: "package-v1"},
			{ID: "provider:package/apt", Revision: "1"},
		},
		Facts: []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}},
	}).WithCanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	for endpoint := 0; endpoint < 400; endpoint++ {
		selected, missing, ok := artifactvariant.SelectHighestCompatible(variants, document)
		if !ok || len(missing) != 0 || selected.SchemaVersion != 1 {
			t.Fatalf("endpoint %d selection = schema %d missing=%+v ok=%t", endpoint, selected.SchemaVersion, missing, ok)
		}
	}
	if len(store.variants) != 2 {
		t.Fatalf("400 selections changed compiled variant cardinality to %d", len(store.variants))
	}
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
    targetDistros: [Debian]
    targetArch: [x86]
    packages:
      - name: curl
        present: true
        packageManager: apt
`)
	store := &capturingVariantStore{}
	service := &CompositionService{RepoRoot: repo, Store: store, ProviderMatrix: qualifiedAPTTestMatrix()}
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

func qualifiedAPTTestMatrix() *providermatrix.Matrix {
	return &providermatrix.Matrix{Version: 1, Rows: []providermatrix.Row{{
		Provider: "package", Distribution: "debian", Release: "12", Architecture: "amd64", Backend: "apt",
		ContractRevision: "v1", Environment: "container", Status: "passing", Selectors: []string{"make:provider-matrix-apt-debian-12"},
	}}}
}
