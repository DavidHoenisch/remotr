package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

func Test_endpointFromRow_mapsFields(t *testing.T) {
	row := db.Endpoint{
		ID:              "stllr-remotr-a1b2c3d4",
		Fleet:           "test-fleet",
		CertFingerprint: pgtype.Text{String: "sha256:abc", Valid: true},
	}
	ep, err := endpointFromRow(row)
	if err != nil {
		t.Fatal(err)
	}
	if ep.ID != "stllr-remotr-a1b2c3d4" {
		t.Fatalf("id = %q", ep.ID)
	}
	if ep.Fleet != "test-fleet" {
		t.Fatalf("fleet = %q", ep.Fleet)
	}
	if ep.CertFingerprint != "sha256:abc" {
		t.Fatalf("fingerprint = %q", ep.CertFingerprint)
	}
}

func Test_parseEndpointID_acceptsLegacyUUID(t *testing.T) {
	want := "33333333-3333-3333-3333-333333333333"
	got, err := parseEndpointID(want)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func Test_parseEndpointID_rejectsInvalid(t *testing.T) {
	if _, err := parseEndpointID("not valid"); err == nil {
		t.Fatal("expected error")
	}
}
