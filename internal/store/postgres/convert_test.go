package postgres

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

func Test_endpointFromRow_mapsFields(t *testing.T) {
	id := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	row := db.Endpoint{
		ID: pgtype.UUID{Bytes: id, Valid: true},
		Fleet: "test-fleet",
		CertFingerprint: pgtype.Text{String: "sha256:abc", Valid: true},
	}
	ep, err := endpointFromRow(row)
	if err != nil {
		t.Fatal(err)
	}
	if ep.ID != id.String() {
		t.Fatalf("id = %q", ep.ID)
	}
	if ep.Fleet != "test-fleet" {
		t.Fatalf("fleet = %q", ep.Fleet)
	}
	if ep.CertFingerprint != "sha256:abc" {
		t.Fatalf("fingerprint = %q", ep.CertFingerprint)
	}
}

func Test_uuidFromString_roundTrip(t *testing.T) {
	want := "33333333-3333-3333-3333-333333333333"
	u, err := uuidFromString(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := uuidString(u)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func Test_uuidFromString_rejectsInvalid(t *testing.T) {
	if _, err := uuidFromString("not-a-uuid"); err == nil {
		t.Fatal("expected error")
	}
}
