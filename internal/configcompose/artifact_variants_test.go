package configcompose_test

import (
	"bytes"
	"path/filepath"
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/artifactrequirements"
	"github.com/DavidHoenisch/remotr/internal/configcompose"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
)

func TestRenderArtifactVariantsIncludesCanonicalSchema1AndLosslessSchema0(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), kindModule(`configurations:
  - name: base
    packages:
      - name: curl
        present: true
        packageManager: apt
`))
	writeFile(t, filepath.Join(dir, "fleets", "engineering", "manifest.yaml"), kindManifest(`modules:
  - modules/base.yaml
`))

	variants, err := configcompose.RenderFleetVariants(dir, "engineering")
	if err != nil {
		t.Fatal(err)
	}
	if len(variants) != 2 {
		t.Fatalf("variant count = %d, want schema 1 and lossless schema 0", len(variants))
	}
	if variants[0].SchemaVersion != 1 || variants[1].SchemaVersion != 0 {
		t.Fatalf("variant schemas = %d, %d", variants[0].SchemaVersion, variants[1].SchemaVersion)
	}
	if !bytes.Contains(variants[0].Artifact, []byte("schemaVersion: 1")) || bytes.Contains(variants[1].Artifact, []byte("schemaVersion:")) {
		t.Fatalf("unexpected schema encodings:\n%s\n%s", variants[0].Artifact, variants[1].Artifact)
	}
	if variants[0].SourceDigest != variants[1].SourceDigest || variants[0].SourceDigest == "" {
		t.Fatalf("source digests = %q and %q", variants[0].SourceDigest, variants[1].SourceDigest)
	}

	schema0, err := models.ParseState(bytes.NewReader(variants[1].Artifact))
	if err != nil {
		t.Fatal(err)
	}
	recanonical, err := resourceregistry.MarshalCanonical(schema0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(recanonical, variants[0].Artifact) {
		t.Fatalf("schema 0 variant is not behaviorally lossless:\n%s\n%s", variants[1].Artifact, variants[0].Artifact)
	}

	wantResource := artifactrequirements.Requirement{ID: "resource:package", Revision: "package-v1"}
	wantProvider := artifactrequirements.Requirement{ID: "provider:package/apt", Revision: "1"}
	for _, variant := range variants {
		if variant.Requirements.ArtifactSchemaVersion != variant.SchemaVersion ||
			!slices.Contains(variant.Requirements.ResourceCapabilities, wantResource) ||
			!slices.Contains(variant.Requirements.ProviderCapabilities, wantProvider) ||
			variant.RequirementDigest == "" || variant.Digest == "" {
			t.Fatalf("variant metadata = %+v", variant)
		}
	}
}
