package apt

import (
	"context"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type Applicator struct {
	Package models.Package
	Exec    executil.Runner
}

func New(pkg models.Package, exec executil.Runner) *Applicator {
	if exec == nil {
		exec = executil.OSRunner{}
	}
	return &Applicator{Package: pkg, Exec: exec}
}

func (a *Applicator) Name() string { return "apt:" + a.Package.Name }

func (a *Applicator) Description() string { return "apt package " + a.Package.Name }

func (a *Applicator) installed() bool {
	_, _, err := a.Exec.Run("dpkg", "-s", a.Package.Name)
	return err == nil
}

func (a *Applicator) State(_ context.Context) (any, bool) {
	inst := a.installed()
	if a.Package.Present {
		return inst, inst
	}
	return inst, !inst
}

func (a *Applicator) Apply(_ context.Context) error {
	_, met := a.State(context.Background())
	if met {
		return appErr.ErrStateAlreadyMet
	}
	if a.Package.Present {
		_, _, err := a.Exec.Run("apt-get", "install", "-y", a.Package.Name)
		return err
	}
	_, _, err := a.Exec.Run("apt-get", "remove", "-y", a.Package.Name)
	return err
}

func (a *Applicator) Revert(_ context.Context) error { return appErr.ErrNoOp }
