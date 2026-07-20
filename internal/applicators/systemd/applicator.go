package systemd

import (
	"context"
	"fmt"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/applicators/systemdctl"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
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

func (a *Applicator) State(ctx context.Context) (any, bool) {
	result := a.Check(ctx)
	return result.Actual, result.Status == executor.Compliant
}

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("systemd unit state " + a.Resource.Unit)
	if err := ctx.Err(); err != nil {
		return systemdCheckFailed(desired, err)
	}
	if a.Resource.Masked != nil {
		masked, err := a.isMasked()
		if err != nil {
			return systemdCheckFailed(desired, err)
		}
		if *a.Resource.Masked != masked {
			return systemdDrifted(desired, "mask state differs", masked)
		}
	}
	if a.Resource.Enabled != nil {
		enabled, err := a.isEnabled()
		if err != nil {
			return systemdCheckFailed(desired, err)
		}
		if *a.Resource.Enabled != enabled {
			return systemdDrifted(desired, "enablement differs", enabled)
		}
	}
	if a.Resource.Active != nil {
		active, err := a.isActive()
		if err != nil {
			return systemdCheckFailed(desired, err)
		}
		if *a.Resource.Active != active {
			return systemdDrifted(desired, "active state differs", active)
		}
	}
	return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "systemd unit state matches", Actual: true}
}

func (a *Applicator) Apply(ctx context.Context) error {
	check := a.Check(ctx)
	if check.Status == executor.Compliant {
		return appErr.ErrStateAlreadyMet
	}
	if check.Status != executor.Drifted {
		if check.Err != nil {
			return check.Err
		}
		return fmt.Errorf("systemd unit cannot apply: %s", check.ObservedSummary)
	}
	if err := systemdctl.DaemonReload(a.Exec); err != nil {
		return err
	}
	if a.Resource.Masked != nil && !*a.Resource.Masked {
		if _, _, err := a.Exec.Run("systemctl", "unmask", a.Resource.Unit); err != nil {
			return err
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
			if err != nil {
				return err
			}
		} else {
			if _, _, err := a.Exec.Run("systemctl", "stop", a.Resource.Unit); err != nil {
				return err
			}
		}
	}
	if a.Resource.Masked != nil && *a.Resource.Masked {
		if _, _, err := a.Exec.Run("systemctl", "mask", a.Resource.Unit); err != nil {
			return err
		}
	}
	return nil
}

func (a *Applicator) Revert(_ context.Context) error { return appErr.ErrNoOp }

func (a *Applicator) isEnabled() (bool, error) {
	out, stderr, err := a.Exec.Run("systemctl", "is-enabled", a.Resource.Unit)
	s := strings.TrimSpace(string(out))
	switch s {
	case "enabled", "enabled-runtime":
		return true, nil
	case "disabled", "static", "indirect", "generated", "transient", "alias", "linked", "linked-runtime", "masked", "masked-runtime":
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("systemctl is-enabled %s: %s: %w", a.Resource.Unit, strings.TrimSpace(string(stderr)), err)
	}
	return false, fmt.Errorf("systemctl is-enabled %s returned unexpected state %q", a.Resource.Unit, s)
}

func (a *Applicator) isActive() (bool, error) {
	out, stderr, err := a.Exec.Run("systemctl", "is-active", a.Resource.Unit)
	s := strings.TrimSpace(string(out))
	switch s {
	case "active":
		return true, nil
	case "inactive", "failed", "activating", "deactivating", "reloading":
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("systemctl is-active %s: %s: %w", a.Resource.Unit, strings.TrimSpace(string(stderr)), err)
	}
	return false, fmt.Errorf("systemctl is-active %s returned unexpected state %q", a.Resource.Unit, s)
}

func (a *Applicator) isMasked() (bool, error) {
	out, stderr, err := a.Exec.Run("systemctl", "is-enabled", a.Resource.Unit)
	s := strings.TrimSpace(string(out))
	switch s {
	case "masked", "masked-runtime":
		return true, nil
	case "enabled", "enabled-runtime", "disabled", "static", "indirect", "generated", "transient", "alias", "linked", "linked-runtime":
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("systemctl is-enabled %s: %s: %w", a.Resource.Unit, strings.TrimSpace(string(stderr)), err)
	}
	return false, fmt.Errorf("systemctl is-enabled %s returned unexpected state %q", a.Resource.Unit, s)
}

func systemdDrifted(desired executor.RedactedSummary, observed string, actual any) executor.CheckResult {
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary(observed), Actual: actual}
}

func systemdCheckFailed(desired executor.RedactedSummary, err error) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, ObservedSummary: "systemd unit check failed", Err: err}
}
