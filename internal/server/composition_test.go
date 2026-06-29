package server

import (
	"context"
	"testing"
	"time"

	pgstore "github.com/DavidHoenisch/remotr/internal/store/postgres"
)

type missArtifactStore struct{}

func (missArtifactStore) StoreCompiledArtifactForFleet(context.Context, string, string, string, []byte, string) error {
	return nil
}
func (missArtifactStore) StoreCompiledArtifactForEndpoint(context.Context, string, string, string, []byte, string) error {
	return nil
}
func (missArtifactStore) GetCompiledArtifactForFleet(context.Context, string, string, string) ([]byte, string, error) {
	return nil, "", pgstore.ErrCompiledArtifactNotFound
}
func (missArtifactStore) GetCompiledArtifactForEndpoint(context.Context, string, string, string) ([]byte, string, error) {
	return nil, "", pgstore.ErrCompiledArtifactNotFound
}
func (missArtifactStore) PruneOldCompiledArtifacts(context.Context, time.Time) error {
	return nil
}

func TestResolveDesiredArtifact_fallsBackToOnDemandRender(t *testing.T) {
	repo := t.TempDir()
	writeTestFleetDesired(t, repo, "lab", `configurations:
  - name: smoke
    commands:
      - name: noop
        apply: [true]
`)

	artifact, digest, err := resolveDesiredArtifact(context.Background(), missArtifactStore{}, repo, "lab", "11111111-1111-1111-1111-111111111111", "any-ref")
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact) == 0 || digest == "" {
		t.Fatalf("artifact=%q digest=%q", artifact, digest)
	}
}
