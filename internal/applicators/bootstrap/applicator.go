package bootstrap

import (
	"context"
	"fmt"
	"os"
	"strings"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type Applicator struct {
	Resource models.BootstrapResource
	Exec     executil.Runner
}

func New(r models.BootstrapResource, exec executil.Runner) *Applicator {
	if exec == nil {
		exec = executil.OSRunner{}
	}
	return &Applicator{Resource: r, Exec: exec}
}

func (a *Applicator) Name() string { return "bootstrap:" + a.Resource.Name }

func (a *Applicator) Description() string {
	when := a.Resource.When
	switch {
	case when.PathMissing != "":
		return fmt.Sprintf("bootstrap %q when %s missing", a.Resource.Name, when.PathMissing)
	case when.PathExists != "":
		return fmt.Sprintf("bootstrap %q when %s exists", a.Resource.Name, when.PathExists)
	default:
		return "bootstrap " + a.Resource.Name
	}
}

// conditionTrue reports whether the When trigger requires running steps.
func (a *Applicator) conditionTrue() bool {
	when := a.Resource.When
	if path := strings.TrimSpace(when.PathMissing); path != "" {
		_, err := os.Stat(path)
		return os.IsNotExist(err)
	}
	if path := strings.TrimSpace(when.PathExists); path != "" {
		_, err := os.Stat(path)
		return err == nil
	}
	return false
}

func (a *Applicator) State(_ context.Context) (any, bool) {
	return nil, !a.conditionTrue()
}

func (a *Applicator) Apply(_ context.Context) error {
	if !a.conditionTrue() {
		return appErr.ErrStateAlreadyMet
	}
	for i, step := range a.Resource.Steps {
		if err := a.applyStep(step); err != nil {
			return fmt.Errorf("bootstrap %q step %d: %w", a.Resource.Name, i+1, err)
		}
	}
	return nil
}

func (a *Applicator) Revert(_ context.Context) error { return appErr.ErrNoOp }

func (a *Applicator) applyStep(step models.BootstrapStep) error {
	switch {
	case step.Systemd != nil:
		return a.applySystemd(*step.Systemd)
	case len(step.Exec) > 0:
		_, _, err := a.Exec.Run(step.Exec[0], step.Exec[1:]...)
		return err
	default:
		return fmt.Errorf("empty step")
	}
}

func (a *Applicator) applySystemd(s models.BootstrapSystemdStep) error {
	if s.Enabled != nil {
		var err error
		if *s.Enabled {
			_, _, err = a.Exec.Run("systemctl", "enable", s.Unit)
		} else {
			_, _, err = a.Exec.Run("systemctl", "disable", s.Unit)
		}
		if err != nil {
			return err
		}
	}
	if s.Active != nil {
		if *s.Active {
			_, _, err := a.Exec.Run("systemctl", "start", s.Unit)
			return err
		}
		_, _, err := a.Exec.Run("systemctl", "stop", s.Unit)
		return err
	}
	return nil
}
