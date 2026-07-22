package configcompose_test

import (
	"bytes"
	"path/filepath"
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/artifactrequirements"
	"github.com/DavidHoenisch/remotr/internal/artifactvariant"
	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
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

// OS-AEC-090: compatibility is evaluated against complete shared variants;
// provider fields and their resources are never removed for one endpoint.
func TestCompositionDoesNotCreateEndpointSpecificPartialVariant(t *testing.T) {
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
	document, err := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{0, 1}, AgentVersion: "v1.2.3",
		Capabilities: []capabilitydoc.Capability{{ID: "resource:package", Revision: "package-v1"}},
		Facts:        []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}},
	}).WithCanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}

	selected, missing, ok := artifactvariant.SelectHighestCompatible(variants, document)
	if ok {
		t.Fatalf("selected endpoint-specific partial variant: %+v", selected)
	}
	if !slices.Contains(missing, artifactvariant.MissingRequirement{ID: "provider:package/apt", Revision: "1"}) {
		t.Fatalf("missing requirements = %+v", missing)
	}
	if len(variants) != 2 {
		t.Fatalf("composition manufactured %d variants", len(variants))
	}
	for _, variant := range variants {
		if !bytes.Contains(variant.Artifact, []byte("name: curl")) || !bytes.Contains(variant.Artifact, []byte("packageManager: apt")) {
			t.Fatalf("variant dropped desired behavior:\n%s", variant.Artifact)
		}
	}
}

func TestRenderMixedTargetVariantsRetainArtifactAndProjectRequirements(t *testing.T) {
	repo := filepath.Join("..", "..", "test", "config-repos", "capability-delivery-blockers")
	variants, err := configcompose.RenderFleetVariants(repo, "engineering")
	if err != nil {
		t.Fatal(err)
	}
	if len(variants) < 4 {
		t.Fatalf("mixed target variant count = %d, want multiple bounded projections", len(variants))
	}
	var artifact []byte
	var digest string
	var ubuntuX86, archX86 *artifactvariant.Variant
	for index := range variants {
		variant := &variants[index]
		if variant.SchemaVersion != 1 {
			continue
		}
		if artifact == nil {
			artifact = variant.Artifact
			digest = variant.Digest
		}
		if !bytes.Equal(variant.Artifact, artifact) || variant.Digest != digest {
			t.Fatalf("target variant changed canonical artifact identity: %+v", variant.Requirements.Target)
		}
		if variant.Requirements.Target == nil {
			continue
		}
		target := variant.Requirements.Target
		if slices.Equal(target.Distros, []string{"ubuntu"}) && slices.Equal(target.Architectures, []string{"x86"}) {
			ubuntuX86 = variant
		}
		if slices.Equal(target.Distros, []string{"arch"}) && slices.Equal(target.Architectures, []string{"x86"}) {
			archX86 = variant
		}
	}
	if ubuntuX86 == nil || archX86 == nil {
		t.Fatalf("target variants omit Ubuntu/x86 or Arch/x86: %+v", variants)
	}
	apt := artifactrequirements.Requirement{ID: "provider:package/apt", Revision: "1"}
	pacman := artifactrequirements.Requirement{ID: "provider:package/pacman", Revision: "1"}
	userFile := artifactrequirements.Requirement{ID: "resource:user-file", Revision: "userFile-v1"}
	if !slices.Contains(ubuntuX86.Requirements.ProviderCapabilities, apt) ||
		slices.Contains(ubuntuX86.Requirements.ProviderCapabilities, pacman) ||
		slices.Contains(ubuntuX86.Requirements.ResourceCapabilities, userFile) {
		t.Fatalf("Ubuntu/x86 requirements = %+v", ubuntuX86.Requirements)
	}
	if !slices.Contains(archX86.Requirements.ProviderCapabilities, pacman) ||
		slices.Contains(archX86.Requirements.ProviderCapabilities, apt) ||
		!slices.Contains(archX86.Requirements.ResourceCapabilities, userFile) {
		t.Fatalf("Arch/x86 requirements = %+v", archX86.Requirements)
	}
}

func BenchmarkRenderMixedTargetArtifactVariants(b *testing.B) {
	repo := filepath.Join("..", "..", "test", "config-repos", "capability-delivery-blockers")
	b.ReportAllocs()
	for b.Loop() {
		variants, err := configcompose.RenderFleetVariants(repo, "engineering")
		if err != nil {
			b.Fatal(err)
		}
		if len(variants) == 0 {
			b.Fatal("no variants")
		}
	}
}
