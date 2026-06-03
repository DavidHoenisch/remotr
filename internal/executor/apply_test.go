package executor

import (
	"context"
	"errors"
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

func TestApplyState_returnsApplyErrorNotRevertNoOp(t *testing.T) {
	applyErr := errors.New("pacman failed")
	h := stubHandler{applyErr: applyErr, revertErr: appErr.ErrNoOp}

	err := New().ApplyState(context.Background(), h)
	if !errors.Is(err, applyErr) {
		t.Fatalf("err = %v, want %v", err, applyErr)
	}
}
