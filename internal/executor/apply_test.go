package executor

import (
	"context"
	"errors"
	"slices"
	"testing"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
)

type stubHandler struct {
	applyErr  error
	revertErr error
}

func (h stubHandler) Name() string        { return "stub" }
func (h stubHandler) Description() string { return "stub" }
func (h stubHandler) State(context.Context) (any, bool) {
	return nil, false
}
func (h stubHandler) Apply(context.Context) error  { return h.applyErr }
func (h stubHandler) Revert(context.Context) error { return h.revertErr }

func TestApplyState_retainsApplyErrorWithNoRollback(t *testing.T) {
	applyErr := errors.New("pacman failed")
	h := stubHandler{applyErr: applyErr, revertErr: appErr.ErrNoOp}

	result := New().ApplyState(context.Background(), h)
	if result.Status != Failed || !errors.Is(result.Err, applyErr) {
		t.Fatalf("result = %+v, want original apply error", result)
	}
	if result.Rollback == nil || result.Rollback.Status != NoRollback {
		t.Fatalf("rollback = %+v, want no-rollback", result.Rollback)
	}
}

func TestApplicationRestartActivationRequiresTargetAndRemainsReportable(t *testing.T) {
	invalid := ApplyResult{Status: Changed, RebootRequired: RebootNotRequired, RollbackClass: RollbackNone, Activation: []ActivationSignal{{Kind: ActivationApplicationRestart}}}
	if err := invalid.Validate(); err == nil {
		t.Fatal("application restart without a target was accepted")
	}
	results := []ApplyResult{{Status: Changed, RebootRequired: RebootNotRequired, RollbackClass: RollbackNone, Activation: []ActivationSignal{
		{Kind: ActivationNextBoot},
		{Kind: ActivationApplicationRestart, Target: "firefox"},
		{Kind: ActivationLogoutRequired},
	}}}
	want := []ActivationSignal{{Kind: ActivationLogoutRequired}, {Kind: ActivationApplicationRestart, Target: "firefox"}, {Kind: ActivationNextBoot}}
	if got := CollectActivations(results); !slices.Equal(got, want) {
		t.Fatalf("CollectActivations() = %v, want %v", got, want)
	}
}
