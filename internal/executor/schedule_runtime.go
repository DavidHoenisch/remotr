package executor

import (
	"context"
	"fmt"
)

// ScheduleRunStatus classifies optional execution-history telemetry from a
// native endpoint scheduler. It is deliberately separate from CheckStatus.
type ScheduleRunStatus string

const (
	ScheduleRunSucceeded ScheduleRunStatus = "succeeded"
	ScheduleRunFailed    ScheduleRunStatus = "failed"
	ScheduleRunRunning   ScheduleRunStatus = "running"
	ScheduleRunSkipped   ScheduleRunStatus = "skipped"
	ScheduleRunUnknown   ScheduleRunStatus = "unknown"
)

// ScheduleMissedRunBehavior reports the installed scheduler's downtime
// policy without claiming that a particular occurrence was observed.
type ScheduleMissedRunBehavior string

const (
	ScheduleMissedRunCatchUp ScheduleMissedRunBehavior = "catch-up"
	ScheduleMissedRunSkip    ScheduleMissedRunBehavior = "skip"
)

// ScheduleRuntimeTelemetry is optional runtime evidence from an endpoint-local
// scheduler. It must never be used to determine configuration compliance.
type ScheduleRuntimeTelemetry struct {
	Status            ScheduleRunStatus
	ExitCode          *int
	MissedRunBehavior ScheduleMissedRunBehavior
}

func (t ScheduleRuntimeTelemetry) Validate() error {
	switch t.Status {
	case ScheduleRunSucceeded, ScheduleRunFailed, ScheduleRunRunning, ScheduleRunSkipped, ScheduleRunUnknown:
	default:
		return fmt.Errorf("executor: unknown schedule run status %q", t.Status)
	}
	switch t.MissedRunBehavior {
	case ScheduleMissedRunCatchUp, ScheduleMissedRunSkip:
	default:
		return fmt.Errorf("executor: unknown schedule missed-run behavior %q", t.MissedRunBehavior)
	}
	return nil
}

// ScheduleRuntimeReporter is implemented only by schedule providers whose
// native backend exposes useful execution history.
type ScheduleRuntimeReporter interface {
	ScheduleRuntime(context.Context) (ScheduleRuntimeTelemetry, bool)
}

// ScheduleRuntime returns valid optional runtime evidence from a handler.
func ScheduleRuntime(ctx context.Context, handler Handler) (ScheduleRuntimeTelemetry, bool) {
	reporter, ok := handler.(ScheduleRuntimeReporter)
	if !ok {
		return ScheduleRuntimeTelemetry{}, false
	}
	telemetry, ok := reporter.ScheduleRuntime(ctx)
	if !ok || telemetry.Validate() != nil {
		return ScheduleRuntimeTelemetry{}, false
	}
	return telemetry, true
}
