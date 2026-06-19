package registry

import "time"

// CronJobStatus is the server view of one cron job for an endpoint.
type CronJobStatus struct {
	Name             string    `json:"name"`
	Schedule         string    `json:"schedule,omitempty"`
	Applicable       bool      `json:"applicable"`
	LastScheduledFor time.Time `json:"last_scheduled_for,omitempty"`
	LastStatus       string    `json:"last_status,omitempty"`
	LastMessage      string    `json:"last_message,omitempty"`
	LastCompletedAt  time.Time `json:"last_completed_at,omitempty"`
}

// CronReport summarizes cron execution state for one endpoint.
type CronReport struct {
	EndpointID  string          `json:"endpoint_id"`
	Fleet       string          `json:"fleet"`
	CronsDigest string          `json:"crons_digest,omitempty"`
	Jobs        []CronJobStatus `json:"jobs"`
}

// CronSummary counts cron health buckets for a fleet.
type CronSummary struct {
	Total      int `json:"total"`
	Applicable int `json:"applicable"`
	Success    int `json:"success"`
	Failed     int `json:"failed"`
	Running    int `json:"running"`
	NeverRun   int `json:"never_run"`
}

// FleetCronReport aggregates cron status for one fleet.
type FleetCronReport struct {
	Fleet     string       `json:"fleet"`
	Summary   CronSummary  `json:"summary"`
	Endpoints []CronReport `json:"endpoints"`
}
