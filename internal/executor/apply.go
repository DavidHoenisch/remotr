package executor

import (
	"context"
)

type Handler interface {
	// name of the applicator
	Name() string

	// Description of applicator
	Description() string

	// Checks if the target state is already met
	State(ctx context.Context) (any, bool)

	// applies given state definition
	// Apply takes a context.
	Apply(ctx context.Context) error

	// reverts a failed state definition
	Revert(ctx context.Context) error
}

type Applicator struct{}

func New() *Applicator {
	return &Applicator{}
}

func (a *Applicator) ApplyState(ctx context.Context, h Handler) error {
	err := h.Apply(ctx)
	if err != nil {
		return h.Revert(ctx)
	}

	return nil
}
