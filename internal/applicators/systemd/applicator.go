package systemd

import (
	"context"
	"strings"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type Applicator struct {
	Resource models.SystemdResource
	Exec     executil.Runner
}

func New(r models.SystemdResource, exec executil.Runner) *Applicator {
	if exec == nil {
		exec = executil.OSRunner{}
	}
	return &Applicator{Resource: r, Exec: exec}
}

func (a *Applicator) Name() string { return "systemd:" + a.Resource.Name }

func (a *Applicator) Description() string { return "systemd unit " + a.Resource.Unit }

func (a *Applicator) State(_ context.Context) (any, bool) {
	if a.Resource.Masked != nil {
		masked, _ := a.isMasked()
		if *a.Resource.Masked != masked {
			return masked, false
		}
	}
	if a.Resource.Enabled != nil {
		enabled, _ := a.isEnabled()
		if *a.Resource.Enabled != enabled {
			return enabled, false
		}
	}
	if a.Resource.Active != nil {
		active, _ := a.isActive()
		if *a.Resource.Active != active {
			return active, false
		}
	}
	return true, true
}

func (a *Applicator) Apply(_ context.Context) error {
	_, met := a.State(context.Background())
	if met {
		return appErr.ErrStateAlreadyMet
	}
	if a.Resource.Masked != nil {
		if *a.Resource.Masked {
			_, _, err := a.Exec.Run("systemctl", "mask", a.Resource.Unit)
			if err != nil {
				return err
			}
		} else {
			_, _, err := a.Exec.Run("systemctl", "unmask", a.Resource.Unit)
			if err != nil {
				return err
			}
		}
	}
	if a.Resource.Enabled != nil {
		var err error
		if *a.Resource.Enabled {
			_, _, err = a.Exec.Run("systemctl", "enable", a.Resource.Unit)
		} else {
			_, _, err = a.Exec.Run("systemctl", "disable", a.Resource.Unit)
		}
		if err != nil {
			return err
		}
	}
	if a.Resource.Active != nil {
		if *a.Resource.Active {
			_, _, err := a.Exec.Run("systemctl", "start", a.Resource.Unit)
			return err
		}
		_, _, err := a.Exec.Run("systemctl", "stop", a.Resource.Unit)
		return err
	}
	return nil
}

func (a *Applicator) Revert(_ context.Context) error { return appErr.ErrNoOp }

func (a *Applicator) isEnabled() (bool, error) {
	out, _, err := a.Exec.Run("systemctl", "is-enabled", a.Resource.Unit)
	if err != nil {
		return false, err
	}
	s := strings.TrimSpace(string(out))
	return s == "enabled" || s == "enabled-runtime", nil
}

func (a *Applicator) isActive() (bool, error) {
	out, _, err := a.Exec.Run("systemctl", "is-active", a.Resource.Unit)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "active", nil
}

func (a *Applicator) isMasked() (bool, error) {
	out, _, err := a.Exec.Run("systemctl", "is-enabled", a.Resource.Unit)
	if err != nil {
		s := strings.TrimSpace(string(out))
		if s == "masked" {
			return true, nil
		}
	}
	s := strings.TrimSpace(string(out))
	return s == "masked", nil
}
