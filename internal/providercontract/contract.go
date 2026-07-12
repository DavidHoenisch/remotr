// Package providercontract defines the behavior boundary used by provider
// conformance tests. It wraps the supported executor.Handler interface rather
// than provider-specific helpers or call-order expectations.
package providercontract

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executor"
)

// CheckStatus classifies the observable result of a provider check.
type CheckStatus string

const (
	Compliant   CheckStatus = "compliant"
	Drifted     CheckStatus = "drifted"
	Unsupported CheckStatus = "unsupported"
	CheckFailed CheckStatus = "failed"
)

// Observation is the typed, provider-independent outcome of Check.
//
// The legacy executor.Handler State method represents only compliant or
// drifted state. Providers that can distinguish unsupported or probe failure
// may implement Provider directly as the conformance suite grows.
type Observation struct {
	Status CheckStatus
	Actual any
	Err    error
}

// ApplyStatus classifies whether Apply mutated the target state.
type ApplyStatus string

const (
	Changed  ApplyStatus = "changed"
	NoChange ApplyStatus = "no-change"
	Failed   ApplyStatus = "failed"
)

// ApplyResult preserves a provider failure while making the no-mutation case
// explicit for idempotence assertions.
type ApplyResult struct {
	Status ApplyStatus
	Err    error
}

// RollbackStatus classifies the currently supported rollback boundary.
type RollbackStatus string

const (
	Reverted       RollbackStatus = "reverted"
	NoRollback     RollbackStatus = "no-rollback"
	RollbackFailed RollbackStatus = "failed"
)

// RollbackResult preserves rollback failures without treating a documented
// no-op rollback as a provider error.
type RollbackResult struct {
	Status RollbackStatus
	Err    error
}

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
	actual, compliant := a.handler.State(ctx)
	if compliant {
		return Observation{Status: Compliant, Actual: actual}
	}
	return Observation{Status: Drifted, Actual: actual}
}

// Apply executes the provider through its public handler interface.
func (a *Adapter) Apply(ctx context.Context) ApplyResult {
	err := a.handler.Apply(ctx)
	switch {
	case err == nil:
		return ApplyResult{Status: Changed}
	case errors.Is(err, appErr.ErrStateAlreadyMet):
		return ApplyResult{Status: NoChange}
	default:
		return ApplyResult{Status: Failed, Err: err}
	}
}

// Rollback runs the provider's supported rollback operation. ErrNoOp expresses
// a documented no-rollback class and is not a failure of the adapter.
func (a *Adapter) Rollback(ctx context.Context) RollbackResult {
	err := a.handler.Revert(ctx)
	switch {
	case err == nil:
		return RollbackResult{Status: Reverted}
	case errors.Is(err, appErr.ErrNoOp):
		return RollbackResult{Status: NoRollback}
	default:
		return RollbackResult{Status: RollbackFailed, Err: err}
	}
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
