//go:build postgres

package postgres

import (
	"context"
	"errors"
	"testing"
)

// Removing an endpoint must also remove its cached override artifacts; before
// this they blocked the delete through a non-cascading foreign key.
func TestPostgresDeleteEndpointRemovesCachedOverrideArtifacts(t *testing.T) {
	ctx := context.Background()
	s, name := reservationStore(t)
	id, fleet, ref := name("override-endpoint"), name("servers"), name("rel")
	if _, err := s.RegisterEndpoint(ctx, id, fleet, name("fp")); err != nil {
		t.Fatal(err)
	}
	if err := s.StoreCompiledArtifactForEndpoint(ctx, id, ref, "desired", []byte("override"), "sha256:override"); err != nil {
		t.Fatal(err)
	}
	removed, err := s.DeleteEndpoint(ctx, id)
	if err != nil || !removed {
		t.Fatalf("DeleteEndpoint() = %v, %v; want removed", removed, err)
	}
	if _, _, err := s.GetCompiledArtifactForEndpoint(ctx, id, ref, "desired"); !errors.Is(err, ErrCompiledArtifactNotFound) {
		t.Fatalf("cached override artifact after endpoint delete: err = %v", err)
	}
	for _, constraint := range []string{"compiled_artifacts_endpoint_id_fkey", "compiled_artifact_variants_endpoint_id_fkey"} {
		var action string
		if err := s.pool.QueryRow(ctx, `SELECT confdeltype::text FROM pg_constraint WHERE conname = $1`, constraint).Scan(&action); err != nil {
			t.Fatalf("%s: %v", constraint, err)
		}
		if action != "c" {
			t.Fatalf("%s on delete action = %q, want cascade", constraint, action)
		}
	}
}
