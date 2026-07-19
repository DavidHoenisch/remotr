package server

import (
	"context"
	"strings"
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

func TestOnDemandArtifactResolverRejectsUnknownArtifactType(t *testing.T) {
	const endpointID = "11111111-1111-1111-1111-111111111111"
	repo := t.TempDir()
	writeTestFleetDesired(t, repo, "lab", "configurations:\n  - name: smoke\n")
	writeTestEndpointOverride(t, repo, endpointID, "configurations:\n  - name: override\n")
	resolver := &OnDemandArtifactResolver{RepoRoot: repo}

	tests := []struct {
		name    string
		resolve func() error
	}{
		{
			name: "fleet",
			resolve: func() error {
				_, _, err := resolver.GetCompiledArtifactForFleet(t.Context(), "lab", "release", "unsupported")
				return err
			},
		},
		{
			name: "endpoint",
			resolve: func() error {
				_, _, err := resolver.GetCompiledArtifactForEndpoint(t.Context(), endpointID, "release", "unsupported")
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.resolve()
			if err == nil || !strings.Contains(err.Error(), `unknown artifact type "unsupported"`) {
				t.Fatalf("error = %v, want unsupported artifact type rejection", err)
			}
		})
	}
}
