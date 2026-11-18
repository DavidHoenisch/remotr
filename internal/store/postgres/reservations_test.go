package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// OS-ENR-009: an override for an ID that is neither enrolled nor reserved
// violates the compiled_artifacts endpoint foreign key at the database boundary.
func TestStoreCompiledArtifactForEndpoint_mapsMissingEndpoint(t *testing.T) {
	fk := &pgconn.PgError{Code: "23503", ConstraintName: "compiled_artifacts_endpoint_id_fkey"}
	store := NewFromQueries(&fakeQuerier{endpointArtifactErr: fk})
	err := store.StoreCompiledArtifactForEndpoint(t.Context(), "unknown-host-01", "rel-1", "desired", []byte("x"), "sha256:x")
	if !errors.Is(err, ErrEndpointNotRegistered) {
		t.Fatalf("err = %v, want ErrEndpointNotRegistered", err)
	}

	other := &pgconn.PgError{Code: "23505"}
	store = NewFromQueries(&fakeQuerier{endpointArtifactErr: other})
	err = store.StoreCompiledArtifactForEndpoint(t.Context(), "known-host-01", "rel-1", "desired", []byte("x"), "sha256:x")
	if errors.Is(err, ErrEndpointNotRegistered) || !errors.As(err, new(*pgconn.PgError)) {
		t.Fatalf("err = %v, want unmapped unique violation", err)
	}
}
