package audit

import (
	"testing"
	"time"
)

func TestCursorRoundTrip(t *testing.T) {
	at := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	id := "11111111-1111-1111-1111-111111111111"

	cursor, err := EncodeCursor(at, id)
	if err != nil {
		t.Fatal(err)
	}
	gotAt, gotID, err := DecodeCursor(cursor)
	if err != nil {
		t.Fatal(err)
	}
	if !gotAt.Equal(at) {
		t.Fatalf("time = %v want %v", gotAt, at)
	}
	if gotID != id {
		t.Fatalf("id = %q", gotID)
	}
}
