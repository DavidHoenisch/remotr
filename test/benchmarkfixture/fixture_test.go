package benchmarkfixture

import (
	"bytes"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/configcompose"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestArtifactIsDeterministicAndParseable(t *testing.T) {
	for _, size := range Sizes() {
		t.Run(size.String(), func(t *testing.T) {
			first := Artifact(size)
			second := Artifact(size)
			if !bytes.Equal(first, second) {
				t.Fatal("Artifact is not deterministic")
			}

			state, err := models.ParseState(bytes.NewReader(first))
			if err != nil {
				t.Fatalf("ParseState() error = %v", err)
			}
			if len(state.Configurations) != 1 || len(state.Configurations[0].Packages) != int(size) {
				t.Fatalf("artifact resources = %d, want %d", len(state.Configurations[0].Packages), size)
			}
		})
	}
}

func TestSchema1ArtifactIsDeterministicAndCarriesSourceEvidence(t *testing.T) {
	for _, size := range Sizes() {
		t.Run(size.String(), func(t *testing.T) {
			first := Schema1Artifact(size)
			second := Schema1Artifact(size)
			if !bytes.Equal(first, second) {
				t.Fatal("Schema1Artifact is not deterministic")
			}

			state, err := models.ParseState(bytes.NewReader(first))
			if err != nil {
				t.Fatalf("ParseState() error = %v", err)
			}
			if state.SchemaVersion != 1 || len(state.Configurations) != 1 || len(state.Configurations[0].Packages) != int(size) {
				t.Fatalf("schema-1 artifact = version:%d configurations:%d resources:%d, want version 1 and %d resources", state.SchemaVersion, len(state.Configurations), len(state.Configurations[0].Packages), size)
			}
			if len(state.ResourceSources) != int(size) {
				t.Fatalf("source evidence count = %d, want %d", len(state.ResourceSources), size)
			}
		})
	}
}

func TestCompositionRepositoryRendersFleetAndEndpoint(t *testing.T) {
	root := WriteCompositionRepository(t, 10)

	for _, target := range []struct {
		name   string
		render func() ([]byte, error)
	}{
		{"fleet", func() ([]byte, error) {
			desired, _, _, _, err := configcompose.RenderFleet(root, "benchmark")
			return desired, err
		}},
		{"endpoint", func() ([]byte, error) {
			desired, _, _, _, err := configcompose.RenderEndpoint(root, "benchmark-endpoint")
			return desired, err
		}},
	} {
		t.Run(target.name, func(t *testing.T) {
			desired, err := target.render()
			if err != nil {
				t.Fatal(err)
			}
			state, err := models.ParseState(bytes.NewReader(desired))
			if err != nil {
				t.Fatal(err)
			}
			if len(state.Configurations) != 1 || len(state.Configurations[0].Packages) != 10 {
				t.Fatalf("resources = %d, want 10", len(state.Configurations[0].Packages))
			}
		})
	}
}
