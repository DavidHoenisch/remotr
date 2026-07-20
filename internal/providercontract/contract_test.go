package providercontract_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/providercontract"
)

func TestAdapterObservesThePublicHandlerContract(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextKey("request"), "value")
	handler := &stubHandler{
		name:        "test-provider",
		description: "test provider",
		state:       "old",
	}
	provider, err := providercontract.New(handler)
	if err != nil {
		t.Fatal(err)
	}

	observation := provider.Check(ctx)
	if observation.Status != providercontract.Drifted || observation.Actual != "old" {
		t.Fatalf("check = %+v, want drifted old state", observation)
	}
	if handler.stateContext != ctx {
		t.Fatal("Check did not pass the caller context to State")
	}

	result := provider.Apply(ctx)
	if result.Status != providercontract.Changed || result.Err != nil {
		t.Fatalf("apply = %+v, want changed without error", result)
	}
	if handler.applyContext != ctx {
		t.Fatal("Apply did not pass the caller context to the provider")
	}
}

func TestAdapterNormalizesAlreadyCompliantApply(t *testing.T) {
	provider, err := providercontract.New(&stubHandler{applyErr: appErr.ErrStateAlreadyMet})
	if err != nil {
		t.Fatal(err)
	}

	result := provider.Apply(context.Background())
	if result.Status != providercontract.NoChange || result.Err != nil {
		t.Fatalf("apply = %+v, want no change without error", result)
	}
}

func TestAdapterPreservesApplyFailureAndRejectsNilHandler(t *testing.T) {
	want := errors.New("runner failed")
	provider, err := providercontract.New(&stubHandler{applyErr: want})
	if err != nil {
		t.Fatal(err)
	}
	if result := provider.Apply(context.Background()); result.Status != providercontract.Failed || !errors.Is(result.Err, want) {
		t.Fatalf("apply = %+v, want preserved failure", result)
	}

	if _, err := providercontract.New(nil); err == nil {
		t.Fatal("New(nil) succeeded")
	}
}

// OS-AEC-010, OS-AEC-011: providers expose one validated, structured Check
// outcome without collapsing probe failures or unavailable capabilities into
// ordinary drift.
func TestObservationValidatesEveryStructuredCheckStatus(t *testing.T) {
	probeFailure := errors.New("probe command exited 1")
	tests := []struct {
		name  string
		check providercontract.Observation
	}{
		{
			name: "compliant",
			check: providercontract.Observation{
				Status:          providercontract.Compliant,
				ReasonCode:      providercontract.ReasonCompliant,
				DesiredSummary:  providercontract.RedactedSummary("version=1.2.3"),
				ObservedSummary: providercontract.RedactedSummary("version=1.2.3"),
			},
		},
		{
			name: "drifted",
			check: providercontract.Observation{
				Status:          providercontract.Drifted,
				ReasonCode:      providercontract.ReasonStateDrift,
				DesiredSummary:  providercontract.RedactedSummary("mode=0640"),
				ObservedSummary: providercontract.RedactedSummary("mode=0600"),
			},
		},
		{
			name: "unsupported",
			check: providercontract.Observation{
				Status:     providercontract.Unsupported,
				ReasonCode: providercontract.ReasonProviderUnavailable,
			},
		},
		{
			name: "check failed",
			check: providercontract.Observation{
				Status:     providercontract.CheckFailed,
				ReasonCode: providercontract.ReasonProbeFailed,
				Err:        probeFailure,
			},
		},
		{
			name: "deferred",
			check: providercontract.Observation{
				Status:     providercontract.Deferred,
				ReasonCode: providercontract.ReasonDeferred,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.check.Validate(); err != nil {
				t.Fatalf("Validate() = %v", err)
			}
		})
	}
}

func TestObservationRejectsUnknownStatusAndUnstableReasonCode(t *testing.T) {
	for _, check := range []providercontract.Observation{
		{Status: providercontract.CheckStatus("unknown"), ReasonCode: providercontract.ReasonCode("unknown")},
		{Status: providercontract.Drifted, ReasonCode: providercontract.ReasonCode("state drift")},
	} {
		if err := check.Validate(); err == nil {
			t.Fatalf("Validate() succeeded for %+v", check)
		}
	}
}

// OS-AEC-010, OS-AEC-011: a handler that implements the structured Check
// contract can expose outcomes that legacy State cannot represent.
func TestAdapterUsesStructuredHandlerCheck(t *testing.T) {
	want := providercontract.Observation{
		Status:          providercontract.Unsupported,
		ReasonCode:      providercontract.ReasonProviderUnavailable,
		DesiredSummary:  providercontract.RedactedSummary("provider=apt"),
		ObservedSummary: providercontract.RedactedSummary("provider=missing"),
	}
	provider, err := providercontract.New(&structuredCheckHandler{
		stubHandler: &stubHandler{state: "legacy drift", compliant: false},
		check:       want,
	})
	if err != nil {
		t.Fatal(err)
	}

	got := provider.Check(context.Background())
	if got.Status != want.Status || got.ReasonCode != want.ReasonCode || got.DesiredSummary != want.DesiredSummary || got.ObservedSummary != want.ObservedSummary {
		t.Fatalf("Check() = %+v, want %+v", got, want)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("structured Check result is invalid: %v", err)
	}
}

// OS-AEC-066: providers can report changed state, deferred follow-up, reboot
// requirements, rollback capability, activation, and redacted diagnostics.
func TestAdapterUsesStructuredHandlerApplyResult(t *testing.T) {
	want := providercontract.ApplyResult{
		Status: providercontract.Changed,
		Activation: []providercontract.ActivationSignal{
			{Kind: providercontract.ActivationDaemonReload},
			{Kind: providercontract.ActivationRestart, Target: "example.service"},
			{Kind: providercontract.ActivationRebootRequired},
		},
		RebootRequired: providercontract.RebootRequired,
		DeferredWork: &providercontract.DeferredWork{
			ReasonCode: providercontract.ReasonDeferred,
			Summary:    providercontract.RedactedSummary("run during next maintenance window"),
		},
		RollbackClass: providercontract.RollbackTransactional,
		Diagnostics:   []providercontract.RedactedSummary{"unit replacement staged"},
	}
	provider, err := providercontract.New(&structuredApplyHandler{
		stubHandler: &stubHandler{},
		result:      want,
	})
	if err != nil {
		t.Fatal(err)
	}

	got := provider.Apply(context.Background())
	if err := got.Validate(); err != nil {
		t.Fatalf("Apply() result is invalid: %v", err)
	}
	if got.Status != want.Status || got.RebootRequired != want.RebootRequired || got.RollbackClass != want.RollbackClass || !slices.Equal(got.Activation, want.Activation) || got.DeferredWork == nil || got.DeferredWork.ReasonCode != providercontract.ReasonDeferred || len(got.Diagnostics) != 1 {
		t.Fatalf("Apply() = %+v, want %+v", got, want)
	}
}

// OS-AEC-067: an Apply failure remains inspectable even when its rollback
// attempt also fails.
func TestAdapterRetainsApplyAndRollbackFailures(t *testing.T) {
	applyErr := errors.New("package transaction failed")
	rollbackErr := errors.New("package database rollback failed")
	provider, err := providercontract.New(&stubHandler{applyErr: applyErr, revertErr: rollbackErr})
	if err != nil {
		t.Fatal(err)
	}

	result := provider.Apply(context.Background())
	if result.Status != providercontract.Failed || !errors.Is(result.Err, applyErr) {
		t.Fatalf("Apply() = %+v, want original apply error", result)
	}
	if result.Rollback == nil || result.Rollback.Status != providercontract.RollbackFailed || !errors.Is(result.Rollback.Err, rollbackErr) {
		t.Fatalf("Apply() rollback = %+v, want retained rollback failure", result.Rollback)
	}
}

type contextKey string

type structuredCheckHandler struct {
	*stubHandler
	check providercontract.Observation
}

func (h *structuredCheckHandler) Check(context.Context) providercontract.Observation {
	return h.check
}

type structuredApplyHandler struct {
	*stubHandler
	result providercontract.ApplyResult
}

func (h *structuredApplyHandler) ApplyResult(context.Context) providercontract.ApplyResult {
	return h.result
}

type stubHandler struct {
	name         string
	description  string
	state        any
	compliant    bool
	applyErr     error
	revertErr    error
	stateContext context.Context
	applyContext context.Context
}

func (h *stubHandler) Name() string        { return h.name }
func (h *stubHandler) Description() string { return h.description }
func (h *stubHandler) State(ctx context.Context) (any, bool) {
	h.stateContext = ctx
	return h.state, h.compliant
}
func (h *stubHandler) Apply(ctx context.Context) error {
	h.applyContext = ctx
	return h.applyErr
}
func (h *stubHandler) Revert(context.Context) error {
	if h.revertErr != nil {
		return h.revertErr
	}
	return appErr.ErrNoOp
}
