package command

import (
	"context"
	"fmt"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type Applicator struct {
	Resource models.CommandResource
	Exec     executil.Runner
}

func New(r models.CommandResource, exec executil.Runner) *Applicator {
	if exec == nil {
		exec = executil.OSRunner{}
	}
	return &Applicator{Resource: r, Exec: exec}
}

func (a *Applicator) Name() string { return "command:" + a.Resource.Name }

func (a *Applicator) Description() string { return "command " + a.Resource.Name }

func (a *Applicator) State(_ context.Context) (any, bool) {
	if len(a.Resource.Check) == 0 {
		return nil, true
	}
	_, _, err := a.Exec.Run(a.Resource.Check[0], a.Resource.Check[1:]...)
	return nil, err == nil
}

func (a *Applicator) Apply(_ context.Context) error {
	_, met := a.State(context.Background())
	if met {
		return appErr.ErrStateAlreadyMet
	}
	if len(a.Resource.Apply) == 0 {
		return fmt.Errorf("command %q: apply not defined", a.Resource.Name)
	}
	_, _, err := a.Exec.Run(a.Resource.Apply[0], a.Resource.Apply[1:]...)
	return err
}

func (a *Applicator) Revert(_ context.Context) error {
	if len(a.Resource.Revert) == 0 {
		return appErr.ErrNoOp
	}
	_, _, err := a.Exec.Run(a.Resource.Revert[0], a.Resource.Revert[1:]...)
	return err
}
