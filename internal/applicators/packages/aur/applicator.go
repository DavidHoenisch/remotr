package aur

import (
	"context"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// Applicator manages packages via pacman (Arch).
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

func (a *Applicator) Name() string { return "pacman:" + a.Package.Name }

func (a *Applicator) Description() string { return "pacman package " + a.Package.Name }

func (a *Applicator) installed() bool {
	_, _, err := a.Exec.Run("pacman", "-Q", a.Package.Name)
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
		_, _, err := a.Exec.Run("pacman", "-S", "--noconfirm", a.Package.Name)
		return err
	}
	_, _, err := a.Exec.Run("pacman", "-R", "--noconfirm", a.Package.Name)
	return err
}

func (a *Applicator) Revert(_ context.Context) error { return appErr.ErrNoOp }
