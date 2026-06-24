package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

func TestEndpointFromRow_reportedUsernames(t *testing.T) {
	t.Parallel()

	stored := interactiveuser.JoinUsernames([]string{"alice", "bob"})
	row := db.Endpoint{
		ID:                "laptop-a",
		Fleet:             "engineering",
		ReportedUsernames: pgtype.Text{String: stored, Valid: true},
	}

	ep, err := endpointFromRow(row)
	if err != nil {
		t.Fatal(err)
	}
	if len(ep.Usernames) != 2 || ep.Usernames[0] != "alice" || ep.Usernames[1] != "bob" {
		t.Fatalf("usernames = %#v", ep.Usernames)
	}
}

func TestUpdateEndpointUsernames_roundTrip(t *testing.T) {
	t.Parallel()

	fq := &fakeQuerier{
		byID: map[string]db.Endpoint{
			"laptop-a": {ID: "laptop-a", Fleet: "engineering"},
		},
	}
	store := &Store{q: fq}

	if err := store.UpdateEndpointUsernames(t.Context(), "laptop-a", []string{"alice", "bob"}); err != nil {
		t.Fatal(err)
	}
	row := fq.byID["laptop-a"]
	if !row.ReportedUsernames.Valid || row.ReportedUsernames.String != "alice,bob" {
		t.Fatalf("stored = %#v", row.ReportedUsernames)
	}
	ep, err := endpointFromRow(row)
	if err != nil {
		t.Fatal(err)
	}
	if len(ep.Usernames) != 2 || ep.Usernames[0] != "alice" {
		t.Fatalf("usernames = %#v", ep.Usernames)
	}
}
