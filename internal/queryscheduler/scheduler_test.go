package queryscheduler_test

import (
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/queryscheduler"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// OS-OSQ-007: deterministic scheduling dispatches due queries, skips in-flight
// runs until stale timeout, respects distro/arch selectors, and skips already
// completed slots.
func TestListDue_DeterministicClockAndSelectors(t *testing.T) {
	now := time.Date(2026, 11, 19, 10, 35, 0, 0, time.UTC)
	slot1030 := time.Date(2026, 11, 19, 10, 30, 0, 0, time.UTC)
	queries := []models.QuerySpec{
		{Name: "due-q", Schedule: "*/30 * * * *", SQL: "SELECT 1", TargetDistros: []types.Distro{types.Ubuntu}},
		{Name: "already-ran", Schedule: "*/30 * * * *", SQL: "SELECT 1"},
		{Name: "in-flight", Schedule: "*/30 * * * *", SQL: "SELECT 1"},
		{Name: "stale-running", Schedule: "*/30 * * * *", SQL: "SELECT 1"},
		{Name: "arch-only", Schedule: "*/30 * * * *", SQL: "SELECT 1", TargetDistros: []types.Distro{types.Arch}},
	}
	lastRuns := map[string]queryscheduler.LastRun{
		"already-ran":   {QueryName: "already-ran", ScheduledFor: slot1030, Status: "success"},
		"in-flight":     {QueryName: "in-flight", ScheduledFor: slot1030, Status: "running", StartedAt: now.Add(-5 * time.Minute)},
		"stale-running": {QueryName: "stale-running", ScheduledFor: slot1030, Status: "running", StartedAt: now.Add(-2 * time.Hour)},
	}

	due := queryscheduler.ListDue(now, queries, lastRuns, types.Ubuntu, types.X86)
	names := map[string]bool{}
	for _, d := range due {
		names[d.Query.Name] = true
		if d.RunID == "" || !d.ScheduledFor.Equal(slot1030) {
			t.Fatalf("unexpected due slot: %+v", d)
		}
	}
	if !names["due-q"] || !names["stale-running"] || names["already-ran"] || names["in-flight"] || names["arch-only"] {
		t.Fatalf("due names = %v, want only due-q and stale-running", names)
	}
}

// OS-OSQ-008: server-side diff canonicalizes row maps and computes exact
// added and removed rows deterministically.
func TestComputeRowDiff_CanonicalAddedAndRemoved(t *testing.T) {
	prev := []map[string]string{
		{"port": "22", "process": "sshd"},
		{"port": "80", "process": "nginx"},
	}
	curr := []map[string]string{
		{"process": "sshd", "port": "22"}, // same row, different map key order
		{"port": "443", "process": "nginx"},
	}

	diff := queryscheduler.ComputeRowDiff(prev, curr)
	if len(diff.Added) != 1 || diff.Added[0]["port"] != "443" {
		t.Fatalf("Added = %+v, want single port=443 row", diff.Added)
	}
	if len(diff.Removed) != 1 || diff.Removed[0]["port"] != "80" {
		t.Fatalf("Removed = %+v, want single port=80 row", diff.Removed)
	}

	// Identical sets yield empty diff.
	same := queryscheduler.ComputeRowDiff(curr, curr)
	if len(same.Added) != 0 || len(same.Removed) != 0 {
		t.Fatalf("identical diff = %+v, want empty", same)
	}
}
