package schedule

import (
	"testing"
	"time"
)

func TestLastDue_weeklyCron(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, loc) // Monday after second Sunday
	after := time.Date(2026, 6, 7, 0, 0, 0, 0, loc)  // previous Sunday

	slot, ok := LastDue("0 0 * * 0", loc, now, after)
	if !ok {
		t.Fatal("expected due slot")
	}
	want := time.Date(2026, 6, 21, 0, 0, 0, 0, loc)
	if !slot.Equal(want) {
		t.Fatalf("slot = %s want %s", slot, want)
	}
}

func TestLastDue_firstRun(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 6, 18, 1, 0, 0, 0, loc)

	slot, ok := LastDue("0 0 * * *", loc, now, time.Time{})
	if !ok {
		t.Fatal("expected due slot")
	}
	want := time.Date(2026, 6, 18, 0, 0, 0, 0, loc)
	if !slot.Equal(want) {
		t.Fatalf("slot = %s want %s", slot, want)
	}
}

func TestMatches_minutelyStep(t *testing.T) {
	tm := time.Date(2026, 6, 18, 12, 15, 0, 0, time.UTC)
	if !Matches("*/15 * * * *", tm) {
		t.Fatal("expected match")
	}
	if Matches("*/15 * * * *", tm.Add(5*time.Minute)) {
		t.Fatal("expected no match")
	}
}
