package dnf

import (
	"context"
	"fmt"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// Applicator is a minimal dnf stub for future Fedora/RHEL support.
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

func (a *Applicator) Name() string { return "dnf:" + a.Package.Name }

func (a *Applicator) Description() string { return "dnf package " + a.Package.Name }

func (a *Applicator) installed() bool {
	_, _, err := a.Exec.Run("rpm", "-q", a.Package.Name)
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
	return fmt.Errorf("dnf apply not implemented for %q", a.Package.Name)
}

func (a *Applicator) Revert(_ context.Context) error { return appErr.ErrNoOp }
