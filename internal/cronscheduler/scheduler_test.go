package cronscheduler

import (
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestListDue_dispatchesAfterMissedSlot(t *testing.T) {
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	jobs := []models.CronJob{{
		Name:     "nightly",
		Schedule: "0 0 * * *",
		Commands: []models.CommandResource{{Name: "run", Apply: []string{"true"}}},
	}}
	lastRuns := map[string]LastRun{
		"nightly": {
			CronName:     "nightly",
			ScheduledFor: time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC),
			Status:       "success",
		},
	}

	due := ListDue(now, jobs, lastRuns)
	if len(due) != 1 {
		t.Fatalf("due = %d", len(due))
	}
	if !due[0].ScheduledFor.Equal(time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("scheduledFor = %s", due[0].ScheduledFor)
	}
}

func TestListDue_redispatchesStaleRunningSameSlot(t *testing.T) {
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	slot := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	jobs := []models.CronJob{{
		Name:     "system-upgrade-debian",
		Schedule: "0 0 * * 0",
		Commands: []models.CommandResource{{Name: "run", Apply: []string{"true"}}},
	}}
	lastRuns := map[string]LastRun{
		"system-upgrade-debian": {
			CronName:     "system-upgrade-debian",
			ScheduledFor: slot,
			Status:       "running",
			StartedAt:    now.Add(-5 * 24 * time.Hour),
		},
	}

	due := ListDue(now, jobs, lastRuns)
	if len(due) != 1 {
		t.Fatalf("due = %+v", due)
	}
	if due[0].CronName != "system-upgrade-debian" {
		t.Fatalf("cron = %q", due[0].CronName)
	}
	if !due[0].ScheduledFor.Equal(slot) {
		t.Fatalf("scheduledFor = %s", due[0].ScheduledFor)
	}
	if due[0].RunID == "" {
		t.Fatal("expected new run id")
	}
}

func TestListDue_skipsRunning(t *testing.T) {
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	jobs := []models.CronJob{{
		Name:     "nightly",
		Schedule: "0 0 * * *",
		Commands: []models.CommandResource{{Name: "run", Apply: []string{"true"}}},
	}}
	lastRuns := map[string]LastRun{
		"nightly": {
			CronName:     "nightly",
			ScheduledFor: time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC),
			Status:       "running",
			StartedAt:    now.Add(-10 * time.Minute),
		},
	}
	if due := ListDue(now, jobs, lastRuns); len(due) != 0 {
		t.Fatalf("due = %+v", due)
	}
}
