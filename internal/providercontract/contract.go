// Package providercontract defines the behavior boundary used by provider
// conformance tests. It wraps the supported executor.Handler interface rather
// than provider-specific helpers or call-order expectations.
package providercontract

import (
	"context"
	"fmt"
	"reflect"

	"github.com/DavidHoenisch/remotr/internal/executor"
)

// CheckStatus classifies the observable result of a provider check.
type CheckStatus = executor.CheckStatus

const (
	Compliant   = executor.Compliant
	Drifted     = executor.Drifted
	Unsupported = executor.Unsupported
	CheckFailed = executor.CheckFailed
	Deferred    = executor.Deferred
)

// ReasonCode is a stable, machine-readable explanation for a Check outcome.
// Provider-specific codes may be added when they use lower snake case.
type ReasonCode = executor.ReasonCode

const (
	ReasonCompliant           = executor.ReasonCompliant
	ReasonStateDrift          = executor.ReasonStateDrift
	ReasonProviderUnavailable = executor.ReasonProviderUnavailable
	ReasonProbeFailed         = executor.ReasonProbeFailed
	ReasonDeferred            = executor.ReasonDeferred
)

// RedactedSummary contains an already-redacted, human-readable state summary.
// Callers must not place secret values in this type.
type RedactedSummary = executor.RedactedSummary

// Observation is the typed, provider-independent outcome of Check.
//
// The legacy executor.Handler State method represents only compliant or
// drifted state. Providers that can distinguish unsupported or probe failure
// may implement Provider directly as the conformance suite grows.
type Observation = executor.CheckResult

// ApplyStatus classifies whether Apply mutated the target state.
type ApplyStatus = executor.ApplyStatus

const (
	Changed       = executor.Changed
	NoChange      = executor.NoChange
	ApplyDeferred = executor.ApplyDeferred
	Failed        = executor.Failed
)

// ApplyResult preserves a provider failure while making the no-mutation case
// explicit for idempotence assertions.
type ApplyResult = executor.ApplyResult

// ActivationKind identifies post-Apply work without executing it implicitly.
type ActivationKind = executor.ActivationKind

const (
	ActivationDaemonReload   = executor.ActivationDaemonReload
	ActivationReload         = executor.ActivationReload
	ActivationTryRestart     = executor.ActivationTryRestart
	ActivationRestart        = executor.ActivationRestart
	ActivationLogoutRequired = executor.ActivationLogoutRequired
	ActivationNextBoot       = executor.ActivationNextBoot
	ActivationRebootRequired = executor.ActivationRebootRequired
)

// ActivationSignal describes one post-Apply activation need.
type ActivationSignal = executor.ActivationSignal

// RebootRequirement makes reboot-needed state visible without initiating a reboot.
type RebootRequirement = executor.RebootRequirement

const (
	RebootNotRequired = executor.RebootNotRequired
	RebootRequired    = executor.RebootRequired
)

// DeferredWork describes a follow-up that could not safely run now.
type DeferredWork = executor.DeferredWork

// RollbackClass describes the recovery guarantee a resource provides.
type RollbackClass = executor.RollbackClass

const (
	RollbackTransactional = executor.RollbackTransactional
	RollbackBestEffort    = executor.RollbackBestEffort
	RollbackNone          = executor.RollbackNone
)

// RollbackStatus classifies the currently supported rollback boundary.
type RollbackStatus = executor.RollbackStatus

const (
	Reverted       = executor.Reverted
	NoRollback     = executor.NoRollback
	RollbackFailed = executor.RollbackFailed
)

// RollbackResult preserves rollback failures without treating a documented
// no-op rollback as a provider error.
type RollbackResult = executor.RollbackResult

// Provider is the public behavior surface consumed by the shared conformance
// harness. Its methods deliberately expose provider outcomes, not command
// assembly or provider-specific implementation details.
type Provider interface {
	Name() string
	Description() string
	Check(context.Context) Observation
	Apply(context.Context) ApplyResult
	Rollback(context.Context) RollbackResult
}

// Adapter exposes an executor.Handler as a Provider. Construct it around the
// handler returned by a provider's supported constructor.
type Adapter struct {
	handler executor.Handler
}

// New constructs a Provider contract adapter for a supported executor handler.
func New(handler executor.Handler) (*Adapter, error) {
	if nilHandler(handler) {
		return nil, fmt.Errorf("provider contract: handler is required")
	}
	return &Adapter{handler: handler}, nil
}

func (a *Adapter) Name() string        { return a.handler.Name() }
func (a *Adapter) Description() string { return a.handler.Description() }

// Check observes the supported handler state without inspecting provider
// internals. Legacy handlers map their boolean state to compliant or drifted.
func (a *Adapter) Check(ctx context.Context) Observation {
	return executor.Check(ctx, a.handler)
}

// Apply executes the provider through its public handler interface.
func (a *Adapter) Apply(ctx context.Context) ApplyResult {
	return executor.New().ApplyState(ctx, a.handler)
}

// Rollback runs the provider's supported rollback operation. ErrNoOp expresses
// a documented no-rollback class and is not a failure of the adapter.
func (a *Adapter) Rollback(ctx context.Context) RollbackResult {
	return executor.Rollback(ctx, a.handler)
}

func nilHandler(handler executor.Handler) bool {
	if handler == nil {
		return true
	}
	value := reflect.ValueOf(handler)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
