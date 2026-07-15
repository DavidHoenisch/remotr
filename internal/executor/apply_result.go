package executor

import (
	"context"
	"errors"
	"fmt"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
)

// ApplyStatus classifies the mutation result of a resource Apply.
type ApplyStatus string

const (
	Changed       ApplyStatus = "changed"
	NoChange      ApplyStatus = "no-change"
	ApplyDeferred ApplyStatus = "deferred"
	Failed        ApplyStatus = "failed"
)

// ActivationKind identifies post-Apply work without executing it implicitly.
type ActivationKind string

const (
	ActivationDaemonReload      ActivationKind = "daemon-reload"
	ActivationReload            ActivationKind = "reload"
	ActivationTryRestart        ActivationKind = "try-restart"
	ActivationRestart           ActivationKind = "restart"
	ActivationLogoutRequired    ActivationKind = "logout-required"
	ActivationNextBoot          ActivationKind = "next-boot"
	ActivationRebootRequired    ActivationKind = "reboot-required"
	ActivationTrustStoreRefresh ActivationKind = "trust-store-refresh"
)

// ActivationSignal describes one post-Apply activation need.
type ActivationSignal struct {
	Kind   ActivationKind
	Target string
}

// RebootRequirement makes reboot-needed state visible without initiating a
// reboot as an incidental side effect of another resource.
type RebootRequirement string

const (
	RebootNotRequired RebootRequirement = "not-required"
	RebootRequired    RebootRequirement = "required"
)

// DeferredWork describes a follow-up that could not safely run now.
type DeferredWork struct {
	ReasonCode ReasonCode
	Summary    RedactedSummary
}

// RollbackClass describes the recovery guarantee a resource provides.
type RollbackClass string

const (
	RollbackTransactional RollbackClass = "transactional"
	RollbackBestEffort    RollbackClass = "best_effort"
	RollbackNone          RollbackClass = "none"
)

// RollbackStatus classifies the outcome of a rollback attempt.
type RollbackStatus string

const (
	Reverted       RollbackStatus = "reverted"
	NoRollback     RollbackStatus = "no-rollback"
	RollbackFailed RollbackStatus = "failed"
)

// RollbackResult preserves the outcome of the recovery attempt separately
// from the error that caused Apply to fail.
type RollbackResult struct {
	Status RollbackStatus
	Err    error
}

// Validate confirms that a rollback outcome is well formed.
func (r RollbackResult) Validate() error {
	switch r.Status {
	case Reverted, NoRollback, RollbackFailed:
	default:
		return fmt.Errorf("executor: unknown rollback status %q", r.Status)
	}
	if r.Status == RollbackFailed && r.Err == nil {
		return errors.New("executor: failed rollback result requires an error")
	}
	return nil
}

// ApplyResult is the structured result of a resource Apply.
type ApplyResult struct {
	Status         ApplyStatus
	Activation     []ActivationSignal
	RebootRequired RebootRequirement
	DeferredWork   *DeferredWork
	RollbackClass  RollbackClass
	Rollback       *RollbackResult
	Diagnostics    []RedactedSummary
	Err            error
}

// Validate confirms that an ApplyResult has a recognized status and carries
// the fields required to explain deferred or failed work safely.
func (r ApplyResult) Validate() error {
	switch r.Status {
	case Changed, NoChange, ApplyDeferred, Failed:
	default:
		return fmt.Errorf("executor: unknown apply status %q", r.Status)
	}
	switch r.RebootRequired {
	case RebootNotRequired, RebootRequired:
	default:
		return fmt.Errorf("executor: unknown reboot requirement %q", r.RebootRequired)
	}
	switch r.RollbackClass {
	case RollbackTransactional, RollbackBestEffort, RollbackNone:
	default:
		return fmt.Errorf("executor: unknown rollback class %q", r.RollbackClass)
	}
	if r.Status == ApplyDeferred && r.DeferredWork == nil {
		return errors.New("executor: deferred result requires deferred work")
	}
	if r.DeferredWork != nil && !isStableReasonCode(r.DeferredWork.ReasonCode) {
		return fmt.Errorf("executor: invalid deferred reason code %q", r.DeferredWork.ReasonCode)
	}
	if r.Status == Failed && r.Err == nil {
		return errors.New("executor: failed result requires an error")
	}
	if r.Rollback != nil {
		if err := r.Rollback.Validate(); err != nil {
			return err
		}
	}
	for _, signal := range r.Activation {
		if !validActivationKind(signal.Kind) {
			return fmt.Errorf("executor: unknown activation kind %q", signal.Kind)
		}
		if (signal.Kind == ActivationReload || signal.Kind == ActivationTryRestart || signal.Kind == ActivationRestart) && signal.Target == "" {
			return fmt.Errorf("executor: activation %q requires a target", signal.Kind)
		}
	}
	return nil
}

func validActivationKind(kind ActivationKind) bool {
	switch kind {
	case ActivationDaemonReload, ActivationReload, ActivationTryRestart, ActivationRestart, ActivationLogoutRequired, ActivationNextBoot, ActivationRebootRequired, ActivationTrustStoreRefresh:
		return true
	default:
		return false
	}
}

// StructuredApplier is implemented by handlers that can report their full
// Apply outcome. Handler.Apply remains supported while handlers migrate.
type StructuredApplier interface {
	ApplyResult(context.Context) ApplyResult
}

// Apply returns a handler's structured outcome. Legacy handlers preserve
// their current mutation semantics and report only the best-effort rollback
// class currently available through Handler.Revert.
func (a *Applicator) Apply(ctx context.Context, handler Handler) ApplyResult {
	if applier, ok := handler.(StructuredApplier); ok {
		return applier.ApplyResult(ctx)
	}
	err := handler.Apply(ctx)
	if err == nil {
		return ApplyResult{Status: Changed, RebootRequired: RebootNotRequired, RollbackClass: RollbackBestEffort}
	}
	if errors.Is(err, appErr.ErrStateAlreadyMet) {
		return ApplyResult{Status: NoChange, RebootRequired: RebootNotRequired, RollbackClass: RollbackBestEffort}
	}
	return ApplyResult{Status: Failed, RebootRequired: RebootNotRequired, RollbackClass: RollbackBestEffort, Err: err}
}

// Rollback invokes the legacy rollback hook and records its outcome without
// conflating it with the preceding Apply error.
func Rollback(ctx context.Context, handler Handler) RollbackResult {
	err := handler.Revert(ctx)
	switch {
	case err == nil:
		return RollbackResult{Status: Reverted}
	case errors.Is(err, appErr.ErrNoOp):
		return RollbackResult{Status: NoRollback}
	default:
		return RollbackResult{Status: RollbackFailed, Err: err}
	}
}
