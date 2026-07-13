package reboots

import (
	"context"
	"fmt"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/rebootstate"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// Probes exposes only the host observations needed to decide whether reboot
// preparation is safe. Implementations must not mutate or reboot the host.
type Probes interface {
	BootID(context.Context) (string, error)
	OnACPower(context.Context) (bool, error)
	ActiveUsers(context.Context) (bool, error)
	ActiveWorkloadInhibitors(context.Context) (bool, error)
}

type Applicator struct {
	Resource models.RebootResource
	Store    *rebootstate.Store
	Probes   Probes
	now      func() time.Time
}

func New(resource models.RebootResource, store *rebootstate.Store, probes Probes, now func() time.Time) *Applicator {
	if now == nil {
		now = time.Now
	}
	return &Applicator{Resource: resource, Store: store, Probes: probes, now: now}
}

func (a *Applicator) Name() string { return "reboot:" + a.Resource.Name }

func (a *Applicator) Description() string {
	return fmt.Sprintf("coordinated reboot generation %s", a.Resource.Generation)
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	result := a.Check(ctx)
	return result.Actual, result.Status == executor.Compliant
}

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	if a.Store == nil || a.Probes == nil {
		return failedCheck("reboot coordinator is unavailable")
	}
	bootID, err := a.Probes.BootID(ctx)
	if err != nil || bootID == "" {
		return failedCheck("boot identity probe failed")
	}
	status, err := a.Store.Snapshot()
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, Err: fmt.Errorf("read reboot state: %w", err)}
	}
	if status.Completion != nil && status.Completion.Generation == a.Resource.Generation && status.Completion.BootID == bootID {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, Actual: status, DesiredSummary: executor.RedactedSummary("reboot generation completed"), ObservedSummary: executor.RedactedSummary("boot identity changed and was verified")}
	}
	if status.Intent != nil {
		if status.Intent.Generation != a.Resource.Generation {
			if status.Intent.Phase == rebootstate.PhaseAwaitingAcknowledgement || status.Intent.Phase == rebootstate.PhaseAttempting {
				return failedCheck("a different reboot generation is already active")
			}
		} else {
			switch status.Intent.Phase {
			case rebootstate.PhaseAwaitingAcknowledgement:
				return deferredCheck("pre_reboot_ack", "awaiting authenticated pre-reboot acknowledgement", status)
			case rebootstate.PhaseAttempting:
				return deferredCheck("reboot_verifying", status.Intent.Reason, status)
			case rebootstate.PhaseTimedOut, rebootstate.PhaseFailed:
				return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonCode(status.Intent.Reason), Actual: status, Err: fmt.Errorf("coordinated reboot %s", status.Intent.Reason)}
			}
		}
	}
	if a.Resource.OnlyIfRequired && !status.Required {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, Actual: status, DesiredSummary: executor.RedactedSummary("reboot only when required"), ObservedSummary: executor.RedactedSummary("no reboot requirement is pending")}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, Actual: status, DesiredSummary: executor.RedactedSummary("coordinated reboot is pending"), ObservedSummary: executor.RedactedSummary("generation has not completed")}
}

func (a *Applicator) Apply(context.Context) error { return nil }

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	if err := a.Resource.Validate(); err != nil {
		return failedApply(err)
	}
	if a.Store == nil || a.Probes == nil {
		return failedApply(errorsafe("reboot coordinator is unavailable"))
	}
	status, err := a.Store.Snapshot()
	if err != nil {
		return failedApply(fmt.Errorf("read reboot state: %w", err))
	}
	if a.Resource.OnlyIfRequired && !status.Required {
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
	}
	now := a.now().UTC()
	deadline := a.Resource.DeadlineTime()
	notBefore := now.Add(a.Resource.DelayDuration())
	if !deadline.IsZero() && !deadline.After(notBefore) {
		return deferredApply("reboot_deadline_elapsed", "reboot deadline has elapsed")
	}
	if window := a.Resource.MaintenanceWindow; window != nil && !window.Contains(now) {
		return deferredApply("maintenance_window", "outside the declared reboot maintenance window")
	}
	if a.Resource.RequireACPower {
		onAC, err := a.Probes.OnACPower(ctx)
		if err != nil {
			return failedApply(errorsafe("AC power probe failed"))
		}
		if !onAC {
			return deferredApply("ac_power_required", "AC power precondition is not met")
		}
	}
	if a.Resource.UserInhibition == models.InhibitionDefer {
		active, err := a.Probes.ActiveUsers(ctx)
		if err != nil {
			return failedApply(errorsafe("active user probe failed"))
		}
		if active {
			return deferredApply("active_user_inhibitor", "an active user inhibits reboot")
		}
	}
	if a.Resource.WorkloadInhibition == models.InhibitionDefer {
		active, err := a.Probes.ActiveWorkloadInhibitors(ctx)
		if err != nil {
			return failedApply(errorsafe("workload inhibitor probe failed"))
		}
		if active {
			return deferredApply("active_workload_inhibitor", "an active workload inhibits reboot")
		}
	}
	bootID, err := a.Probes.BootID(ctx)
	if err != nil || bootID == "" {
		return failedApply(errorsafe("boot identity probe failed"))
	}
	prepared, err := a.Store.Prepare(rebootstate.Intent{
		Generation: a.Resource.Generation, Phase: rebootstate.PhaseAwaitingAcknowledgement,
		PriorBootID: bootID, PreparedAt: now, NotBefore: notBefore,
		Timeout: a.Resource.TimeoutDuration(), Deadline: deadline,
	})
	if err != nil {
		return failedApply(err)
	}
	if prepared.Completion != nil && prepared.Completion.Generation == a.Resource.Generation {
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
	}
	return deferredApply("pre_reboot_ack", "reboot intent prepared; awaiting authenticated Sync acknowledgement")
}

func (a *Applicator) Revert(context.Context) error { return appErr.ErrNoOp }

func deferredCheck(reason executor.ReasonCode, summary string, actual any) executor.CheckResult {
	if summary == "" {
		summary = string(reason)
	}
	return executor.CheckResult{Status: executor.Deferred, ReasonCode: reason, Actual: actual, ObservedSummary: executor.RedactedSummary(summary)}
}

func failedCheck(message string) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, Err: errorsafe(message)}
}

func deferredApply(reason executor.ReasonCode, summary string) executor.ApplyResult {
	return executor.ApplyResult{
		Status: executor.ApplyDeferred, RebootRequired: executor.RebootRequired, RollbackClass: executor.RollbackNone,
		DeferredWork: &executor.DeferredWork{ReasonCode: reason, Summary: executor.RedactedSummary(summary)},
	}
}

func failedApply(err error) executor.ApplyResult {
	return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone, Err: err}
}

func errorsafe(message string) error { return fmt.Errorf("%s", message) }
