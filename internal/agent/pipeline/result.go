package pipeline

import (
	"github.com/DavidHoenisch/remotr/internal/agent/engine"
)

// Result captures pipeline outcomes for sync telemetry.
type Result struct {
	Labels       map[string]string
	Drift        engine.DriftReport
	Apply        engine.ApplyResult
	ApplyFailure *engine.ApplyFailure
}
