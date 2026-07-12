package providercontract_test

import (
	"context"
	"errors"
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

type contextKey string

type stubHandler struct {
	name         string
	description  string
	state        any
	compliant    bool
	applyErr     error
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
func (h *stubHandler) Revert(context.Context) error { return appErr.ErrNoOp }
