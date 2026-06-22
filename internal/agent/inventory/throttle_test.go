package inventory

import (
	"testing"
	"time"
)

func TestThrottlerShouldReport(t *testing.T) {
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	th := NewThrottler(time.Hour, 5*time.Minute)

	if !th.ShouldReport(now, "abc") {
		t.Fatal("first report should be allowed")
	}

	th.MarkSent(now, "abc")

	if th.ShouldReport(now.Add(time.Minute), "abc") {
		t.Fatal("should not report within interval with same digest")
	}
	if th.ShouldReport(now.Add(time.Minute), "def") {
		t.Fatal("should not report change within change min gap")
	}
	if !th.ShouldReport(now.Add(6*time.Minute), "def") {
		t.Fatal("should report change after change min gap")
	}

	th.MarkSent(now.Add(6*time.Minute), "def")
	if !th.ShouldReport(now.Add(66*time.Minute), "def") {
		t.Fatal("should report after interval elapsed")
	}
}

func TestThrottlerLoadState(t *testing.T) {
	sent := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	th := NewThrottler(time.Hour, 5*time.Minute)
	th.LoadState(ThrottleState{LastSentAt: sent, LastSentDigest: "xyz"})

	if th.LastSentDigest != "xyz" {
		t.Fatalf("digest = %q", th.LastSentDigest)
	}
	st := th.State()
	if !st.LastSentAt.Equal(sent) {
		t.Fatalf("lastSentAt = %v", st.LastSentAt)
	}
}
