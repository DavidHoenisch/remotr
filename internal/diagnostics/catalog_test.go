package diagnostics

import (
	"testing"
	"time"
)

func TestNormalizeCollectors(t *testing.T) {
	t.Parallel()
	got, err := NormalizeCollectors(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(allCollectors) {
		t.Fatalf("default collectors = %d, want %d", len(got), len(allCollectors))
	}

	_, err = NormalizeCollectors([]string{"bad_collector"})
	if err == nil {
		t.Fatal("expected error for invalid collector")
	}

	got, err = NormalizeCollectors([]string{"system_info", "system_info", "dmesg"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("dedup = %v", got)
	}
}

func TestNormalizeTimeRange(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	since, until, err := NormalizeTimeRange(time.Time{}, time.Time{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if until != now {
		t.Fatalf("until = %v", until)
	}
	if !since.Equal(now.Add(-DefaultLookback)) {
		t.Fatalf("since = %v", since)
	}

	_, _, err = NormalizeTimeRange(now.Add(-8*24*time.Hour), now, now)
	if err == nil {
		t.Fatal("expected span error")
	}

	_, _, err = NormalizeTimeRange(now, now.Add(time.Hour), now)
	if err == nil {
		t.Fatal("expected future until error")
	}
}
