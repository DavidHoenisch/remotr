package cronscheduler

import (
	"time"

	"github.com/google/uuid"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/schedule"
)

const runningTimeout = 4 * time.Hour

// LastRun is the persisted execution state for one endpoint cron.
type LastRun struct {
	CronName     string
	RunID        string
	ScheduledFor time.Time
	Status       string
	StartedAt    time.Time
	CompletedAt  time.Time
	Message      string
	UpdatedAt    time.Time
}

// DueJob is a cron job the server wants the agent to execute.
type DueJob struct {
	RunID        string
	CronName     string
	ScheduledFor time.Time
	Job          models.CronJob
}

// ListDue returns cron jobs that should run on the next agent check-in.
func ListDue(now time.Time, jobs []models.CronJob, lastRuns map[string]LastRun) []DueJob {
	out := make([]DueJob, 0, len(jobs))
	for _, job := range jobs {
		if due, ok := dueJob(now, job, lastRuns[job.Name]); ok {
			out = append(out, due)
		}
	}
	return out
}

func dueJob(now time.Time, job models.CronJob, last LastRun) (DueJob, bool) {
	if last.Status == "running" {
		started := last.StartedAt
		if started.IsZero() {
			started = last.UpdatedAt
		}
		if now.Sub(started) < runningTimeout {
			return DueJob{}, false
		}
	}

	loc, err := schedule.LocationForCron(job.Timezone)
	if err != nil {
		return DueJob{}, false
	}

	anchor := time.Time{}
	if !last.ScheduledFor.IsZero() && (last.Status == "success" || last.Status == "failed") {
		anchor = last.ScheduledFor
	}

	slot, ok := schedule.LastDue(job.Schedule, loc, now, anchor)
	if !ok {
		return DueJob{}, false
	}
	// Success on this slot is done. Stale "running" (past runningTimeout) is
	// re-dispatched above; in-flight runs are suppressed earlier in dueJob.
	if last.ScheduledFor.Equal(slot) && last.Status == "success" {
		return DueJob{}, false
	}

	return DueJob{
		RunID:        uuid.NewString(),
		CronName:     job.Name,
		ScheduledFor: slot,
		Job:          job,
	}, true
}
