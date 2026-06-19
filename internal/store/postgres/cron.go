package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/cronscheduler"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

// CronResultPayload is agent-reported cron execution telemetry.
type CronResultPayload struct {
	RunID       string          `json:"runId"`
	CronName    string          `json:"cronName"`
	Status      string          `json:"status"`
	StartedAt   time.Time       `json:"startedAt,omitempty"`
	CompletedAt time.Time       `json:"completedAt,omitempty"`
	Message     string          `json:"message,omitempty"`
	Failures    json.RawMessage `json:"failures,omitempty"`
}

// RecordCronResults persists agent cron execution results.
func (s *Store) RecordCronResults(ctx context.Context, endpointID, releaseRef, cronsDigest string, results []CronResultPayload) error {
	if len(results) == 0 {
		return nil
	}
	endpointID, err := parseEndpointID(endpointID)
	if err != nil {
		return err
	}
	for _, result := range results {
		if err := s.recordCronResult(ctx, endpointID, releaseRef, cronsDigest, result); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) recordCronResult(ctx context.Context, endpointID, releaseRef, cronsDigest string, result CronResultPayload) error {
	if result.CronName == "" || result.RunID == "" || result.Status == "" {
		return nil
	}
	runID, err := uuid.Parse(result.RunID)
	if err != nil {
		return err
	}
	details, _ := json.Marshal(map[string]any{"failures": result.Failures})

	if err := s.q.InsertCronExecution(ctx, db.InsertCronExecutionParams{
		ID:           newUUID(),
		EndpointID:   endpointID,
		CronName:     result.CronName,
		CronsDigest:  cronsDigest,
		ReleaseRef:   releaseRef,
		RunID:        pgtype.UUID{Bytes: runID, Valid: true},
		ScheduledFor: timestamptzFromTime(result.StartedAt),
		StartedAt:    timestamptzFromTime(result.StartedAt),
		CompletedAt:  timestamptzFromTime(result.CompletedAt),
		Status:       result.Status,
		Message:      result.Message,
		DetailsJson:  details,
	}); err != nil {
		return err
	}

	existing, err := s.q.GetCronLastRun(ctx, db.GetCronLastRunParams{
		EndpointID: endpointID,
		CronName:   result.CronName,
	})
	scheduledFor := result.StartedAt
	if err == nil && existing.ScheduledFor.Valid {
		scheduledFor = existing.ScheduledFor.Time
	}

	return s.q.UpsertCronLastRun(ctx, db.UpsertCronLastRunParams{
		EndpointID:   endpointID,
		CronName:     result.CronName,
		CronsDigest:  cronsDigest,
		RunID:        pgtype.UUID{Bytes: runID, Valid: true},
		ScheduledFor: timestamptzFromTime(scheduledFor),
		Status:       result.Status,
		StartedAt:    timestamptzFromTime(result.StartedAt),
		CompletedAt:  timestamptzFromTime(result.CompletedAt),
		Message:      result.Message,
	})
}

// MarkCronRunning records a dispatched cron job.
func (s *Store) MarkCronRunning(ctx context.Context, endpointID, cronsDigest string, due cronscheduler.DueJob, startedAt time.Time) error {
	endpointID, err := parseEndpointID(endpointID)
	if err != nil {
		return err
	}
	runID, err := uuid.Parse(due.RunID)
	if err != nil {
		return err
	}
	return s.q.UpsertCronLastRun(ctx, db.UpsertCronLastRunParams{
		EndpointID:   endpointID,
		CronName:     due.CronName,
		CronsDigest:  cronsDigest,
		RunID:        pgtype.UUID{Bytes: runID, Valid: true},
		ScheduledFor: timestamptzFromTime(due.ScheduledFor),
		Status:       "running",
		StartedAt:    timestamptzFromTime(startedAt),
		CompletedAt:  pgtype.Timestamptz{},
		Message:      "",
	})
}

// ListCronLastRuns returns persisted cron state for an endpoint.
func (s *Store) ListCronLastRuns(ctx context.Context, endpointID string) (map[string]cronscheduler.LastRun, error) {
	endpointID, err := parseEndpointID(endpointID)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListCronLastRunsForEndpoint(ctx, endpointID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]cronscheduler.LastRun, len(rows))
	for _, row := range rows {
		out[row.CronName] = lastRunFromRow(row)
	}
	return out, nil
}

func lastRunFromRow(row db.CronLastRun) cronscheduler.LastRun {
	runID := ""
	if row.RunID.Valid {
		runID = uuid.UUID(row.RunID.Bytes).String()
	}
	return cronscheduler.LastRun{
		CronName:     row.CronName,
		RunID:        runID,
		ScheduledFor: timeFromTimestamptz(row.ScheduledFor),
		Status:       row.Status,
		StartedAt:    timeFromTimestamptz(row.StartedAt),
		CompletedAt:  timeFromTimestamptz(row.CompletedAt),
		Message:      row.Message,
		UpdatedAt:    timeFromTimestamptz(row.UpdatedAt),
	}
}

func timestamptzFromTime(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

func timeFromTimestamptz(v pgtype.Timestamptz) time.Time {
	if !v.Valid {
		return time.Time{}
	}
	return v.Time.UTC()
}

// GetEndpointCronReport builds cron status for admin queries.
func (s *Store) GetEndpointCronReport(ctx context.Context, endpointID string, jobs []registry.CronJobStatus) (registry.CronReport, bool, error) {
	ep, err := s.endpointByID(ctx, endpointID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return registry.CronReport{}, false, nil
		}
		return registry.CronReport{}, false, err
	}
	lastRuns, err := s.ListCronLastRuns(ctx, endpointID)
	if err != nil {
		return registry.CronReport{}, false, err
	}
	report := registry.CronReport{
		EndpointID: ep.ID,
		Fleet:      ep.Fleet,
		Jobs:       mergeCronJobStatus(jobs, lastRuns),
	}
	return report, true, nil
}

func mergeCronJobStatus(jobs []registry.CronJobStatus, lastRuns map[string]cronscheduler.LastRun) []registry.CronJobStatus {
	out := make([]registry.CronJobStatus, 0, len(jobs))
	for _, job := range jobs {
		if last, ok := lastRuns[job.Name]; ok {
			job.LastScheduledFor = last.ScheduledFor
			job.LastStatus = last.Status
			job.LastMessage = last.Message
			job.LastCompletedAt = last.CompletedAt
		} else if job.Applicable {
			job.LastStatus = "never"
		}
		out = append(out, job)
	}
	return out
}

// ListFleetCronReports aggregates cron status for a fleet.
func (s *Store) ListFleetCronReports(ctx context.Context, fleet string, jobsForEndpoint func(endpointID string, labels map[string]string) []registry.CronJobStatus) (registry.FleetCronReport, error) {
	endpoints, err := s.ListEndpoints(ctx)
	if err != nil {
		return registry.FleetCronReport{}, err
	}
	out := registry.FleetCronReport{Fleet: fleet}
	for _, ep := range endpoints {
		if ep.Fleet != fleet {
			continue
		}
		jobs := jobsForEndpoint(ep.ID, ep.Labels)
		report, ok, err := s.GetEndpointCronReport(ctx, ep.ID, jobs)
		if err != nil {
			return registry.FleetCronReport{}, err
		}
		if !ok {
			continue
		}
		out.Endpoints = append(out.Endpoints, report)
		out.Summary.Total++
		for _, job := range report.Jobs {
			if !job.Applicable {
				continue
			}
			out.Summary.Applicable++
			switch job.LastStatus {
			case "success":
				out.Summary.Success++
			case "failed":
				out.Summary.Failed++
			case "running":
				out.Summary.Running++
			case "never", "":
				out.Summary.NeverRun++
			}
		}
	}
	return out, nil
}
