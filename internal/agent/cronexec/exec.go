package cronexec

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/resolve"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// Failure is one resource failure during cron execution.
type Failure struct {
	ResourceAddress string `json:"resourceAddress"`
	Message         string `json:"message"`
}

// Result is the outcome of running one cron job.
type Result struct {
	CronName    string
	RunID       string
	Status      string
	Message     string
	Failures    []Failure
	StartedAt   time.Time
	CompletedAt time.Time
}

// Run executes one cron job spec (YAML bytes containing a single crons[] entry).
func Run(ctx context.Context, specYAML []byte, cronName, runID string, exec executil.Runner) Result {
	started := time.Now().UTC()
	out := Result{
		CronName:  cronName,
		RunID:     runID,
		Status:    "success",
		StartedAt: started,
	}

	state, err := models.ParseCronState(bytes.NewReader(specYAML))
	if err != nil {
		out.Status = "failed"
		out.Message = fmt.Sprintf("parse cron spec: %v", err)
		out.CompletedAt = time.Now().UTC()
		return out
	}
	if len(state.Crons) != 1 {
		out.Status = "failed"
		out.Message = fmt.Sprintf("expected one cron job, got %d", len(state.Crons))
		out.CompletedAt = time.Now().UTC()
		return out
	}
	job := state.Crons[0]

	f, err := facts.Read()
	if err != nil {
		out.Status = "failed"
		out.Message = fmt.Sprintf("read facts: %v", err)
		out.CompletedAt = time.Now().UTC()
		return out
	}

	resolved := resolve.Resolve(models.State{Configurations: []models.Configuration{job.ToConfiguration()}}, f)
	eng, err := engine.New(resolved, f, exec, nil)
	if err != nil {
		out.Status = "failed"
		out.Message = fmt.Sprintf("build engine: %v", err)
		out.CompletedAt = time.Now().UTC()
		return out
	}

	result := eng.ApplyAll(ctx, engine.PolicyAuto)
	if result.Failed != nil {
		out.Status = "failed"
		out.Message = result.Failed.Err.Error()
		out.Failures = []Failure{{
			ResourceAddress: result.Failed.Address,
			Message:         result.Failed.Err.Error(),
		}}
	}
	out.CompletedAt = time.Now().UTC()
	return out
}
