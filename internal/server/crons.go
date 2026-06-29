package server

import (
	"bytes"
	"context"
	"log/slog"
	"time"

	"github.com/DavidHoenisch/remotr/internal/croncatalog"
	"github.com/DavidHoenisch/remotr/internal/cronresolve"
	"github.com/DavidHoenisch/remotr/internal/cronscheduler"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/registry"
	pgstore "github.com/DavidHoenisch/remotr/internal/store/postgres"
)

// CronScheduler persists and evaluates server-managed cron jobs.
type CronScheduler interface {
	RecordCronResults(ctx context.Context, endpointID, releaseRef, cronsDigest string, results []pgstore.CronResultPayload) error
	MarkCronRunning(ctx context.Context, endpointID, cronsDigest string, due cronscheduler.DueJob, startedAt time.Time) error
	ListCronLastRuns(ctx context.Context, endpointID string) (map[string]cronscheduler.LastRun, error)
	GetEndpointCronReport(ctx context.Context, endpointID string, jobs []registry.CronJobStatus) (registry.CronReport, bool, error)
	ListFleetCronReports(ctx context.Context, fleet string, jobsForEndpoint func(endpointID string, labels map[string]string) []registry.CronJobStatus) (registry.FleetCronReport, error)
}

type cronResultPayload struct {
	RunID       string          `json:"runId"`
	CronName    string          `json:"cronName"`
	Status      string          `json:"status"`
	StartedAt   time.Time       `json:"startedAt,omitempty"`
	CompletedAt time.Time       `json:"completedAt,omitempty"`
	Message     string          `json:"message,omitempty"`
	Failures    jsonFailures    `json:"failures,omitempty"`
}

type jsonFailures []cronFailurePayload

type cronFailurePayload struct {
	ResourceAddress string `json:"resourceAddress"`
	Message         string `json:"message"`
}

type dueCronPayload struct {
	RunID        string `json:"runId"`
	CronName     string `json:"cronName"`
	ScheduledFor time.Time `json:"scheduledFor"`
	SpecYAML     []byte `json:"specYaml"`
}

func (s *Server) loadResolvedCrons(fleet, endpointID string) ([]models.CronJob, string, bool, error) {
	releaseRef := s.releaseRef(context.Background())
	yamlBytes, digest, ok, err := resolveCronsArtifact(context.Background(), s.cfg.ArtifactStore, s.cfg.ConfigRepoPath, fleet, endpointID, releaseRef)
	if err != nil || !ok {
		return nil, "", ok, err
	}
	state, err := models.ParseCronState(bytes.NewReader(yamlBytes))
	if err != nil {
		return nil, digest, true, err
	}
	resolved, err := croncatalog.Resolve(s.cfg.ConfigRepoPath, state)
	if err != nil {
		return nil, digest, true, err
	}
	return resolved.Crons, digest, true, nil
}

func (s *Server) persistCronResults(ctx context.Context, endpointID, releaseRef, cronsDigest string, req syncRequest) {
	if s.cfg.CronScheduler == nil || len(req.CronResults) == 0 {
		return
	}
	results := make([]pgstore.CronResultPayload, 0, len(req.CronResults))
	for _, item := range req.CronResults {
		results = append(results, pgstore.CronResultPayload{
			RunID:       item.RunID,
			CronName:    item.CronName,
			Status:      item.Status,
			StartedAt:   item.StartedAt,
			CompletedAt: item.CompletedAt,
			Message:     item.Message,
		})
	}
	if err := s.cfg.CronScheduler.RecordCronResults(ctx, endpointID, releaseRef, cronsDigest, results); err != nil {
		slog.Warn("persist cron results", "endpoint", endpointID, "err", err)
	}
}

func (s *Server) dueCronsForEndpoint(ctx context.Context, endpointID, fleet string, labels map[string]string) ([]dueCronPayload, string) {
	if s.cfg.CronScheduler == nil {
		return nil, ""
	}
	jobs, digest, ok, err := s.loadResolvedCrons(fleet, endpointID)
	if err != nil {
		slog.Warn("load crons artifact", "endpoint", endpointID, "err", err)
		return nil, ""
	}
	if !ok || len(jobs) == 0 {
		return nil, ""
	}

	applicable := cronresolve.FilterJobsByLabels(jobs, labels)
	if len(applicable) == 0 {
		return nil, digest
	}
	if labels["distro"] == "" || labels["arch"] == "" {
		return nil, digest
	}

	lastRuns, err := s.cfg.CronScheduler.ListCronLastRuns(ctx, endpointID)
	if err != nil {
		slog.Warn("list cron last runs", "endpoint", endpointID, "err", err)
		return nil, digest
	}

	now := time.Now().UTC()
	dueJobs := cronscheduler.ListDue(now, applicable, lastRuns)
	out := make([]dueCronPayload, 0, len(dueJobs))
	for _, due := range dueJobs {
		spec, err := croncatalog.JobSpecYAML(due.Job)
		if err != nil {
			slog.Warn("encode cron spec", "cron", due.CronName, "err", err)
			continue
		}
		if err := s.cfg.CronScheduler.MarkCronRunning(ctx, endpointID, digest, due, now); err != nil {
			slog.Warn("mark cron running", "cron", due.CronName, "err", err)
			continue
		}
		out = append(out, dueCronPayload{
			RunID:        due.RunID,
			CronName:     due.CronName,
			ScheduledFor: due.ScheduledFor,
			SpecYAML:     spec,
		})
	}
	return out, digest
}

func buildCronJobStatuses(jobs []models.CronJob, labels map[string]string) []registry.CronJobStatus {
	applicable := cronresolve.FilterJobsByLabels(jobs, labels)
	applicableNames := map[string]struct{}{}
	for _, job := range applicable {
		applicableNames[job.Name] = struct{}{}
	}
	out := make([]registry.CronJobStatus, 0, len(jobs))
	for _, job := range jobs {
		_, ok := applicableNames[job.Name]
		out = append(out, registry.CronJobStatus{
			Name:       job.Name,
			Schedule:   job.Schedule,
			Applicable: ok || len(labels) == 0,
		})
	}
	return out
}

func (s *Server) cronJobStatuses(fleet, endpointID string, labels map[string]string) []registry.CronJobStatus {
	jobs, _, ok, err := s.loadResolvedCrons(fleet, endpointID)
	if err != nil || !ok {
		return nil
	}
	return buildCronJobStatuses(jobs, labels)
}
